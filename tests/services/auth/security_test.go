package auth_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/thaletto/krcrackers-go/src/services/auth"
)

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
