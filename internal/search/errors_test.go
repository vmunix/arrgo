// internal/search/errors_test.go
package search

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestErrors(t *testing.T) {
	// Verify errors are distinct
	require.NotErrorIs(t, ErrProwlarrUnavailable, ErrInvalidAPIKey, "ErrProwlarrUnavailable should not equal ErrInvalidAPIKey")
	require.NotErrorIs(t, ErrInvalidAPIKey, ErrNoResults, "ErrInvalidAPIKey should not equal ErrNoResults")

	// Verify error messages
	assert.NotEmpty(t, ErrProwlarrUnavailable.Error(), "ErrProwlarrUnavailable should have a message")
	assert.NotEmpty(t, ErrInvalidAPIKey.Error(), "ErrInvalidAPIKey should have a message")
	assert.NotEmpty(t, ErrNoResults.Error(), "ErrNoResults should have a message")
}
