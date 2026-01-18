// internal/search/errors_test.go
package search

import (
	"errors"
	"testing"
)

func TestErrors(t *testing.T) {
	// Verify errors are distinct
	if errors.Is(ErrProwlarrUnavailable, ErrInvalidAPIKey) {
		t.Error("ErrProwlarrUnavailable should not equal ErrInvalidAPIKey")
	}
	if errors.Is(ErrInvalidAPIKey, ErrNoResults) {
		t.Error("ErrInvalidAPIKey should not equal ErrNoResults")
	}

	// Verify error messages
	if ErrProwlarrUnavailable.Error() == "" {
		t.Error("ErrProwlarrUnavailable should have a message")
	}
	if ErrInvalidAPIKey.Error() == "" {
		t.Error("ErrInvalidAPIKey should have a message")
	}
	if ErrNoResults.Error() == "" {
		t.Error("ErrNoResults should have a message")
	}
}
