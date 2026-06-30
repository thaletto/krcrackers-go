package errors_test

import (
	"errors"
	"fmt"
	"testing"

	apperrors "github.com/thaletto/krcrackers-go/src/errors"
)

func TestErrNotFoundIsStableSentinel(t *testing.T) {
	if apperrors.ErrNotFound == nil {
		t.Fatal("ErrNotFound must not be nil")
	}
	if apperrors.ErrNotFound.Error() != "not found" {
		t.Errorf("message: got %q, want %q", apperrors.ErrNotFound.Error(), "not found")
	}
}

func TestErrNotFoundIsMatchableWithErrorsIs(t *testing.T) {
	wrapped := fmt.Errorf("user %d: %w", 42, apperrors.ErrNotFound)
	if !errors.Is(wrapped, apperrors.ErrNotFound) {
		t.Fatal("errors.Is should match wrapped ErrNotFound")
	}
}

func TestUnrelatedErrorDoesNotMatch(t *testing.T) {
	other := errors.New("something else")
	if errors.Is(other, apperrors.ErrNotFound) {
		t.Fatal("errors.Is should not match an unrelated error")
	}
}
