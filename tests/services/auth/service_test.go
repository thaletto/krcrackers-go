package auth_test

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/thaletto/krcrackers-go/src/database"
	"github.com/thaletto/krcrackers-go/src/migrations"
	"github.com/thaletto/krcrackers-go/src/services/auth"
)

func newAuthService(t *testing.T, verifier auth.GoogleTokenVerifier) *auth.Service {
	t.Helper()

	db, err := database.New(database.Config{
		Mode:  database.ModeLocal,
		Local: &database.LocalConfig{Path: filepath.Join(t.TempDir(), "auth.sqlite")},
	})
	if err != nil {
		t.Fatalf("database.New: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	if _, err := migrations.Up(context.Background(), db); err != nil {
		t.Fatalf("migrations.Up: %v", err)
	}

	return auth.NewServiceWithGoogleVerifier(auth.NewRepository(db), "test-secret", verifier)
}

func identityVerifier(identity auth.GoogleIdentity, err error) auth.GoogleTokenVerifier {
	return func(string) (auth.GoogleIdentity, error) { return identity, err }
}

func TestGoogleLoginCreatesAndReturnsTheSameUser(t *testing.T) {
	ctx := context.Background()
	svc := newAuthService(t, identityVerifier(auth.GoogleIdentity{
		Subject: "google-subject-1",
		Email:   "customer@example.com",
		Name:    "Customer",
	}, nil))

	created, err := svc.LoginWithGoogle(ctx, "valid-token")
	if err != nil {
		t.Fatalf("first LoginWithGoogle: %v", err)
	}
	if created.User.AuthProvider != "google" || created.User.Email != "customer@example.com" {
		t.Fatalf("created user: %+v", created.User)
	}
	if created.AccessToken == "" || created.RefreshToken == "" {
		t.Fatal("expected a session token pair")
	}

	returning, err := svc.LoginWithGoogle(ctx, "valid-token")
	if err != nil {
		t.Fatalf("returning LoginWithGoogle: %v", err)
	}
	if returning.User.ID != created.User.ID {
		t.Fatalf("returning user ID: got %d, want %d", returning.User.ID, created.User.ID)
	}
}

func TestGoogleLoginRejectsPasswordAccountCollision(t *testing.T) {
	ctx := context.Background()
	svc := newAuthService(t, identityVerifier(auth.GoogleIdentity{
		Subject: "google-subject-2",
		Email:   "customer@example.com",
		Name:    "Customer",
	}, nil))
	if _, err := svc.Register(ctx, "customer@example.com", "password123", "Customer", "1234567890"); err != nil {
		t.Fatalf("Register: %v", err)
	}

	_, err := svc.LoginWithGoogle(ctx, "valid-token")
	if !errors.Is(err, auth.ErrGoogleAccountLinkRequired) {
		t.Fatalf("LoginWithGoogle error: got %v, want ErrGoogleAccountLinkRequired", err)
	}
}

func TestGoogleLoginRejectsInvalidIdentity(t *testing.T) {
	svc := newAuthService(t, identityVerifier(auth.GoogleIdentity{}, errors.New("invalid audience")))
	_, err := svc.LoginWithGoogle(context.Background(), "wrong-audience-token")
	if !errors.Is(err, auth.ErrInvalidGoogleToken) {
		t.Fatalf("LoginWithGoogle error: got %v, want ErrInvalidGoogleToken", err)
	}
}

func TestGoogleLoginRequiresConfiguredClientID(t *testing.T) {
	_, err := auth.VerifyGoogleIDToken("unused", "")
	if !errors.Is(err, auth.ErrGoogleLoginUnavailable) {
		t.Fatalf("VerifyGoogleIDToken error: got %v, want ErrGoogleLoginUnavailable", err)
	}
}
