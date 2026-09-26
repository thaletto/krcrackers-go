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

// An email that already has a password account used to be refused outright,
// which locked the customer out of Google sign-in forever. Linking keeps both
// methods working on the same user row.
func TestGoogleLoginLinksAnExistingPasswordAccount(t *testing.T) {
	ctx := context.Background()
	svc := newAuthService(t, identityVerifier(auth.GoogleIdentity{
		Subject: "google-subject-2",
		Email:   "customer@example.com",
		Name:    "Customer",
	}, nil))
	registered, err := svc.Register(ctx, "customer@example.com", "password123", "Customer", "1234567890")
	if err != nil {
		t.Fatalf("Register: %v", err)
	}

	linked, err := svc.LoginWithGoogle(ctx, "valid-token")
	if err != nil {
		t.Fatalf("LoginWithGoogle: %v", err)
	}
	if linked.User.ID != registered.User.ID {
		t.Fatalf("linked into user ID %d, want the registered user %d", linked.User.ID, registered.User.ID)
	}
	if linked.AccessToken == "" || linked.RefreshToken == "" {
		t.Fatal("expected a session token pair")
	}

	// Linking must not cost the customer their password.
	if _, err := svc.Login(ctx, "customer@example.com", "password123"); err != nil {
		t.Fatalf("password Login after linking: %v", err)
	}
	// A wrong password must still be rejected, so linking is not a backdoor.
	if _, err := svc.Login(ctx, "customer@example.com", "wrong-password"); !errors.Is(err, auth.ErrInvalidCredentials) {
		t.Fatalf("wrong-password Login error: got %v, want ErrInvalidCredentials", err)
	}
}

// Linking is idempotent: a repeat Google login resolves through the stored
// subject rather than attempting a second link.
func TestGoogleLoginIsIdempotentAfterLinking(t *testing.T) {
	ctx := context.Background()
	svc := newAuthService(t, identityVerifier(auth.GoogleIdentity{
		Subject: "google-subject-3",
		Email:   "customer@example.com",
		Name:    "Customer",
	}, nil))
	if _, err := svc.Register(ctx, "customer@example.com", "password123", "Customer", "1234567890"); err != nil {
		t.Fatalf("Register: %v", err)
	}
	first, err := svc.LoginWithGoogle(ctx, "valid-token")
	if err != nil {
		t.Fatalf("first LoginWithGoogle: %v", err)
	}
	second, err := svc.LoginWithGoogle(ctx, "valid-token")
	if err != nil {
		t.Fatalf("second LoginWithGoogle: %v", err)
	}
	if first.User.ID != second.User.ID {
		t.Fatalf("user IDs diverged: %d then %d", first.User.ID, second.User.ID)
	}
}

// A different Google identity claiming an already-linked email is ambiguous
// and must be refused rather than silently taking over the account.
func TestGoogleLoginRejectsASecondSubjectForALinkedEmail(t *testing.T) {
	ctx := context.Background()
	// One service, one database: the subject has to change while the link from
	// the first login is still in place.
	subject := "google-subject-4"
	svc := newAuthService(t, func(string) (auth.GoogleIdentity, error) {
		return auth.GoogleIdentity{Subject: subject, Email: "customer@example.com", Name: "Customer"}, nil
	})
	if _, err := svc.Register(ctx, "customer@example.com", "password123", "Customer", "1234567890"); err != nil {
		t.Fatalf("Register: %v", err)
	}
	if _, err := svc.LoginWithGoogle(ctx, "valid-token"); err != nil {
		t.Fatalf("LoginWithGoogle: %v", err)
	}

	subject = "google-subject-5"
	_, err := svc.LoginWithGoogle(ctx, "valid-token")
	if !errors.Is(err, auth.ErrGoogleAccountLinkRequired) {
		t.Fatalf("LoginWithGoogle error: got %v, want ErrGoogleAccountLinkRequired", err)
	}
}

// A Google-only account has no password hash, so it must not be reachable with
// any password.
func TestGoogleOnlyAccountHasNoPasswordLogin(t *testing.T) {
	ctx := context.Background()
	svc := newAuthService(t, identityVerifier(auth.GoogleIdentity{
		Subject: "google-subject-6",
		Email:   "customer@example.com",
		Name:    "Customer",
	}, nil))
	if _, err := svc.LoginWithGoogle(ctx, "valid-token"); err != nil {
		t.Fatalf("LoginWithGoogle: %v", err)
	}
	if _, err := svc.Login(ctx, "customer@example.com", "password123"); !errors.Is(err, auth.ErrInvalidCredentials) {
		t.Fatalf("Login error: got %v, want ErrInvalidCredentials", err)
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
