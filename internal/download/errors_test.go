package download

import (
	"errors"
	"testing"
)

func TestErrors(t *testing.T) {
	// Verify errors are distinct
	if errors.Is(ErrClientUnavailable, ErrInvalidAPIKey) {
		t.Error("ErrClientUnavailable should not equal ErrInvalidAPIKey")
	}
	if errors.Is(ErrDownloadNotFound, ErrNotFound) {
		t.Error("ErrDownloadNotFound should not equal ErrNotFound")
	}

	// Verify error messages are non-empty
	errs := []error{ErrClientUnavailable, ErrInvalidAPIKey, ErrDownloadNotFound, ErrNotFound}
	for _, err := range errs {
		if err.Error() == "" {
			t.Errorf("error %v should have a message", err)
		}
	}
}
