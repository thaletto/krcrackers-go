package auth_test

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/thaletto/krcrackers-go/src/database"
	"github.com/thaletto/krcrackers-go/src/migrations"
	"github.com/thaletto/krcrackers-go/src/services/auth"
)

func newAuthFixture(t *testing.T) (database.DB, int) {
	t.Helper()
	ctx := context.Background()

	db, err := database.New(database.Config{
		Mode:  database.ModeLocal,
		Local: &database.LocalConfig{Path: filepath.Join(t.TempDir(), "auth.sqlite")},
	})
	if err != nil {
		t.Fatalf("database.New: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if _, err := migrations.Up(ctx, db); err != nil {
		t.Fatalf("migrations.Up: %v", err)
	}
	if _, err := db.Execute(ctx, `
		INSERT INTO users (id, email, name, phone, auth_provider, password_hash, role)
		VALUES (1, 'shopper@example.com', 'Shopper', '9000000001', 'email', '', 'customer')
	`); err != nil {
		t.Fatalf("insert user: %v", err)
	}
	return db, 1
}

// The bug this guards: the refresh_tokens table held the token itself — the same
// string the browser has in its cookie. A read of that table (a backup, a dump,
// any future query) yielded a week of bearer credentials for every account,
// while the password column beside it sat bcrypt'd.
func TestRefreshTokensAreNotStoredInTheClear(t *testing.T) {
	ctx := context.Background()
	db, userID := newAuthFixture(t)
	repo := auth.NewRepository(db)

	const token = "a-refresh-token-value"
	if err := repo.CreateRefreshToken(ctx, userID, token, time.Now().Add(time.Hour)); err != nil {
		t.Fatalf("CreateRefreshToken: %v", err)
	}

	rows, err := db.Query(ctx, `SELECT token FROM refresh_tokens`)
	if err != nil {
		t.Fatalf("read refresh_tokens: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("got %d rows, want 1", len(rows))
	}
	stored, _ := rows[0].String("token")
	if stored == token {
		t.Fatal("the refresh token is stored verbatim; a read of this table would hand over live credentials")
	}
	if len(stored) != 64 {
		t.Errorf("stored value is %d chars, want a 64-char hex digest", len(stored))
	}

	// It still resolves, which is the point of hashing.
	uid, expiresAt, err := repo.GetRefreshToken(ctx, token)
	if err != nil {
		t.Fatalf("GetRefreshToken: %v", err)
	}
	if uid != userID {
		t.Errorf("resolved user %d, want %d", uid, userID)
	}
	if time.Until(expiresAt) <= 0 {
		t.Errorf("resolved expiry %v is not in the future", expiresAt)
	}
}

// A token issued before hashing was introduced is stored verbatim. It must keep
// working, and using it must upgrade the row rather than leaving plaintext
// behind forever.
func TestALegacyPlaintextTokenStillWorksAndIsUpgraded(t *testing.T) {
	ctx := context.Background()
	db, userID := newAuthFixture(t)
	repo := auth.NewRepository(db)

	const legacy = "legacy-plaintext-token"
	if _, err := db.Execute(ctx, `
		INSERT INTO refresh_tokens (user_id, token, expires_at) VALUES (?, ?, ?)
	`, userID, legacy, time.Now().Add(time.Hour).UTC().Format("2006-01-02 15:04:05")); err != nil {
		t.Fatalf("plant a legacy row: %v", err)
	}

	uid, _, err := repo.GetRefreshToken(ctx, legacy)
	if err != nil {
		t.Fatalf("GetRefreshToken(legacy): %v", err)
	}
	if uid != userID {
		t.Errorf("legacy token resolved user %d, want %d", uid, userID)
	}

	rows, err := db.Query(ctx, `SELECT token FROM refresh_tokens`)
	if err != nil {
		t.Fatalf("read refresh_tokens: %v", err)
	}
	stored, _ := rows[0].String("token")
	if stored == legacy {
		t.Error("the legacy token was not upgraded; it is still stored in the clear")
	}

	// And it still works after the upgrade.
	if uid, _, err := repo.GetRefreshToken(ctx, legacy); err != nil || uid != userID {
		t.Errorf("GetRefreshToken after the upgrade = %d, %v", uid, err)
	}
}

// The bug this guards: a time was handed to the adapter as a bare time.Time, so
// SQLite wrote the local wall clock and the reader parsed it back as UTC. In
// IST that made a token expiring at 00:30 stay live until 00:30 UTC — five and a
// half hours past its life. The stored value is now explicit UTC.
func TestRefreshExpiryIsJudgedInUTC(t *testing.T) {
	ctx := context.Background()
	db, userID := newAuthFixture(t)
	repo := auth.NewRepository(db)

	expires := time.Now().Add(time.Hour)
	if err := repo.CreateRefreshToken(ctx, userID, "live", expires); err != nil {
		t.Fatalf("CreateRefreshToken: %v", err)
	}
	// The token is stored hashed, so it cannot be selected by name; the fixture
	// holds exactly one row.
	rows, err := db.Query(ctx, `SELECT expires_at FROM refresh_tokens`)
	if err != nil {
		t.Fatalf("read expires_at: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("got %d rows, want 1", len(rows))
	}
	stored, _ := rows[0].String("expires_at")
	if strings.ContainsAny(stored, "T+") || strings.Contains(stored, " m=") {
		t.Errorf("stored expiry %q carries a zone or monotonic suffix; it must be plain UTC", stored)
	}
	if _, err := time.Parse("2006-01-02 15:04:05", stored); err != nil {
		t.Errorf("stored expiry %q does not parse back: %v", stored, err)
	}

	_, got, err := repo.GetRefreshToken(ctx, "live")
	if err != nil {
		t.Fatalf("GetRefreshToken: %v", err)
	}
	// The expiry must land within a second of what was asked for. Before the fix
	// it was off by the local UTC offset.
	if delta := got.Sub(expires.UTC().Truncate(time.Second)); delta > time.Second || delta < -time.Second {
		t.Errorf("expiry came back as %v, want about %v (off by %v)", got, expires.UTC(), delta)
	}
}

func TestAnExpiredRefreshTokenIsRejected(t *testing.T) {
	ctx := context.Background()
	db, userID := newAuthFixture(t)
	repo := auth.NewRepository(db)

	if err := repo.CreateRefreshToken(ctx, userID, "stale", time.Now().Add(-time.Minute)); err != nil {
		t.Fatalf("CreateRefreshToken: %v", err)
	}
	_, expiresAt, err := repo.GetRefreshToken(ctx, "stale")
	if err != nil {
		t.Fatalf("GetRefreshToken: %v", err)
	}
	if !time.Now().After(expiresAt) {
		t.Errorf("a token that expired a minute ago reads as live until %v", expiresAt)
	}
}

// A password policy is the floor under an unthrottled login endpoint. bcrypt
// hashes a single character as readily as a long one, so without a bound the
// only thing standing between an attacker and an account is guessing.
func TestPasswordLengthBounds(t *testing.T) {
	for _, c := range []struct {
		name     string
		password string
		want     error
	}{
		{"too short", "short", auth.ErrPasswordTooShort},
		{"empty", "", auth.ErrPasswordTooShort},
		{"at the limit", strings.Repeat("a", auth.MaxPasswordLength), nil},
		{"one over the limit", strings.Repeat("a", auth.MaxPasswordLength+1), auth.ErrPasswordTooLong},
	} {
		t.Run(c.name, func(t *testing.T) {
			if err := auth.ValidatePassword(c.password); !errors.Is(err, c.want) {
				t.Fatalf("ValidatePassword(%d chars) = %v, want %v", len(c.password), err, c.want)
			}
			// Hashing must refuse the same inputs, and must never return a hash
			// alongside an error.
			hash, err := auth.HashPassword(c.password)
			if c.want == nil {
				if err != nil {
					t.Fatalf("HashPassword: %v", err)
				}
				if !auth.CheckPassword(c.password, hash) {
					t.Error("the hash does not verify against its own password")
				}
				return
			}
			if !errors.Is(err, c.want) {
				t.Fatalf("HashPassword = %v, want %v", err, c.want)
			}
			if hash != "" {
				t.Error("HashPassword returned a hash alongside an error")
			}
		})
	}
}
