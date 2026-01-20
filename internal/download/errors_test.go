package download

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestErrors(t *testing.T) {
	// Verify errors are distinct
	assert.False(t, errors.Is(ErrClientUnavailable, ErrInvalidAPIKey),
		"ErrClientUnavailable should not equal ErrInvalidAPIKey")
	assert.False(t, errors.Is(ErrDownloadNotFound, ErrNotFound),
		"ErrDownloadNotFound should not equal ErrNotFound")

	// Verify error messages are non-empty
	errs := []error{ErrClientUnavailable, ErrInvalidAPIKey, ErrDownloadNotFound, ErrNotFound}
	for _, err := range errs {
		assert.NotEmpty(t, err.Error(), "error %v should have a message", err)
	}
}
