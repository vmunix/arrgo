// internal/search/errors_test.go
package search

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestErrors(t *testing.T) {
	// Verify errors are distinct
	assert.False(t, errors.Is(ErrProwlarrUnavailable, ErrInvalidAPIKey), "ErrProwlarrUnavailable should not equal ErrInvalidAPIKey")
	assert.False(t, errors.Is(ErrInvalidAPIKey, ErrNoResults), "ErrInvalidAPIKey should not equal ErrNoResults")

	// Verify error messages
	assert.NotEmpty(t, ErrProwlarrUnavailable.Error(), "ErrProwlarrUnavailable should have a message")
	assert.NotEmpty(t, ErrInvalidAPIKey.Error(), "ErrInvalidAPIKey should have a message")
	assert.NotEmpty(t, ErrNoResults.Error(), "ErrNoResults should have a message")
}
