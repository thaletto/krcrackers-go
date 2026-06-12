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
	Sub     string `json:"sub"`
	Email   string `json:"email"`
	Name    string `json:"name"`
	Picture string `json:"picture"`
	jwt.RegisteredClaims
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

	var jwks jwksResponse
	if err := json.NewDecoder(resp.Body).Decode(&jwks); err != nil {
		return nil, fmt.Errorf("decode jwks: %w", err)
	}

	keys := make(map[string]*rsa.PublicKey)
	for _, k := range jwks.Keys {
		if k.Kty != "RSA" {
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

func VerifyGoogleIDToken(idToken string) (*googleClaims, error) {
	keys, err := fetchJWKS()
	if err != nil {
		return nil, err
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
	})
	if err != nil {
		return nil, fmt.Errorf("invalid token: %w", err)
	}

	claims, ok := token.Claims.(*googleClaims)
	if !ok || !token.Valid {
		return nil, fmt.Errorf("invalid token claims")
	}

	if claims.Issuer != googleIssuer && claims.Issuer != "accounts.google.com" {
		return nil, fmt.Errorf("invalid issuer: %s", claims.Issuer)
	}

	return claims, nil
}
