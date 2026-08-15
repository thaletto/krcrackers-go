package auth

import (
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"math/big"
	"net/http"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

const (
	googleJWKSURL = "https://www.googleapis.com/oauth2/v3/certs"
	googleIssuer  = "https://accounts.google.com"
)

var httpClient = &http.Client{Timeout: 10 * time.Second}

type jwk struct {
	Kid string `json:"kid"`
	Kty string `json:"kty"`
	Alg string `json:"alg"`
	Use string `json:"use"`
	N   string `json:"n"`
	E   string `json:"e"`
}

type jwksResponse struct {
	Keys []jwk `json:"keys"`
}

type googleClaims struct {
	Email         string `json:"email"`
	Name          string `json:"name"`
	EmailVerified bool   `json:"email_verified"`
	jwt.RegisteredClaims
}

// GoogleIdentity is the verified subset of a Google ID token used for login.
type GoogleIdentity struct {
	Subject string
	Email   string
	Name    string
}

type keyCache struct {
	mu        sync.RWMutex
	keys      map[string]*rsa.PublicKey
	fetchedAt time.Time
}

var cache = &keyCache{keys: make(map[string]*rsa.PublicKey)}

func fetchJWKS() (map[string]*rsa.PublicKey, error) {
	cache.mu.RLock()
	if time.Since(cache.fetchedAt) < 6*time.Hour && len(cache.keys) > 0 {
		keys := cache.keys
		cache.mu.RUnlock()
		return keys, nil
	}
	cache.mu.RUnlock()

	cache.mu.Lock()
	defer cache.mu.Unlock()

	if time.Since(cache.fetchedAt) < 6*time.Hour && len(cache.keys) > 0 {
		return cache.keys, nil
	}

	resp, err := httpClient.Get(googleJWKSURL)
	if err != nil {
		return nil, fmt.Errorf("fetch jwks: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("fetch jwks: unexpected status %d", resp.StatusCode)
	}

	var jwks jwksResponse
	if err := json.NewDecoder(resp.Body).Decode(&jwks); err != nil {
		return nil, fmt.Errorf("decode jwks: %w", err)
	}

	keys := make(map[string]*rsa.PublicKey)
	for _, k := range jwks.Keys {
		if k.Kty != "RSA" || k.Alg != "RS256" || k.Use != "sig" {
			continue
		}
		nBytes, err := base64.RawURLEncoding.DecodeString(k.N)
		if err != nil {
			continue
		}
		eBytes, err := base64.RawURLEncoding.DecodeString(k.E)
		if err != nil {
			continue
		}
		n := new(big.Int).SetBytes(nBytes)
		e := new(big.Int).SetBytes(eBytes)
		keys[k.Kid] = &rsa.PublicKey{N: n, E: int(e.Int64())}
	}

	cache.keys = keys
	cache.fetchedAt = time.Now()
	return keys, nil
}

// VerifyGoogleIDToken validates a Google ID token using Google's JWKS endpoint.
// Keys are cached for 6 hours to reduce latency and avoid rate limits.
func VerifyGoogleIDToken(idToken, clientID string) (GoogleIdentity, error) {
	if clientID == "" {
		return GoogleIdentity{}, ErrGoogleLoginUnavailable
	}
	keys, err := fetchJWKS()
	if err != nil {
		return GoogleIdentity{}, err
	}

	token, err := jwt.ParseWithClaims(idToken, &googleClaims{}, func(t *jwt.Token) (interface{}, error) {
		kid, ok := t.Header["kid"].(string)
		if !ok {
			return nil, fmt.Errorf("missing kid in token header")
		}
		key, exists := keys[kid]
		if !exists {
			cache.mu.Lock()
			cache.fetchedAt = time.Time{}
			cache.mu.Unlock()
			keys, err = fetchJWKS()
			if err != nil {
				return nil, err
			}
			key, exists = keys[kid]
			if !exists {
				return nil, fmt.Errorf("unknown kid: %s", kid)
			}
		}
		return key, nil
	}, jwt.WithValidMethods([]string{jwt.SigningMethodRS256.Alg()}), jwt.WithAudience(clientID))
	if err != nil {
		return GoogleIdentity{}, fmt.Errorf("invalid token: %w", err)
	}

	claims, ok := token.Claims.(*googleClaims)
	if !ok || !token.Valid {
		return GoogleIdentity{}, fmt.Errorf("invalid token claims")
	}
	if claims.Issuer != googleIssuer && claims.Issuer != "accounts.google.com" {
		return GoogleIdentity{}, fmt.Errorf("invalid issuer: %s", claims.Issuer)
	}
	if claims.Subject == "" || claims.Email == "" || !claims.EmailVerified {
		return GoogleIdentity{}, fmt.Errorf("missing verified Google identity")
	}

	return GoogleIdentity{
		Subject: claims.Subject,
		Email:   claims.Email,
		Name:    claims.Name,
	}, nil
}
