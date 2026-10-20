package auth

import (
	"errors"
	"fmt"

	"golang.org/x/crypto/bcrypt"
)

// MinPasswordLength is the shortest password registration accepts. bcrypt hashes
// a single character as readily as twelve, so without a floor the only thing
// standing between an attacker and an account is guessing.
const MinPasswordLength = 12

// MaxPasswordLength is bcrypt's limit: it silently ignores anything past the
// first 72 bytes, so a longer password would be no stronger than its first 72
// bytes. It is checked explicitly so the caller hears about it.
const MaxPasswordLength = 72

var (
	// ErrPasswordTooShort is returned when a password is under MinPasswordLength.
	ErrPasswordTooShort = fmt.Errorf("password must be at least %d characters", MinPasswordLength)

	// ErrPasswordTooLong is returned when a password exceeds bcrypt's 72-byte
	// limit. Without this check bcrypt returns an opaque error that surfaces as
	// a 500.
	ErrPasswordTooLong = fmt.Errorf("password must be at most %d bytes", MaxPasswordLength)
)

// ValidatePassword enforces the length bounds. It is a client mistake, so the
// HTTP layer reports it as a 422 rather than a server fault.
func ValidatePassword(password string) error {
	if len(password) < MinPasswordLength {
		return ErrPasswordTooShort
	}
	if len(password) > MaxPasswordLength {
		return ErrPasswordTooLong
	}
	return nil
}

// HashPassword hashes a plaintext password using bcrypt with the default cost.
func HashPassword(password string) (string, error) {
	if err := ValidatePassword(password); err != nil {
		return "", err
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		if errors.Is(err, bcrypt.ErrPasswordTooLong) {
			return "", ErrPasswordTooLong
		}
		return "", err
	}
	return string(hash), nil
}

// CheckPassword compares a plaintext password against a bcrypt hash.
func CheckPassword(password, hash string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) == nil
}
