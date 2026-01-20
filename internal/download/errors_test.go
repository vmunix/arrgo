package download

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestErrors(t *testing.T) {
	// Verify errors are distinct
	require.NotErrorIs(t, ErrClientUnavailable, ErrInvalidAPIKey,
		"ErrClientUnavailable should not equal ErrInvalidAPIKey")
	require.NotErrorIs(t, ErrDownloadNotFound, ErrNotFound,
		"ErrDownloadNotFound should not equal ErrNotFound")

	// Verify error messages are non-empty
	errs := []error{ErrClientUnavailable, ErrInvalidAPIKey, ErrDownloadNotFound, ErrNotFound}
	for _, err := range errs {
		assert.NotEmpty(t, err.Error(), "error %v should have a message", err)
	}
}
