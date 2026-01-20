package library

import (
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestErrors_AreDistinct(t *testing.T) {
	assert.False(t, errors.Is(ErrNotFound, ErrDuplicate), "ErrNotFound should not match ErrDuplicate")
	assert.False(t, errors.Is(ErrNotFound, ErrConstraint), "ErrNotFound should not match ErrConstraint")
	assert.False(t, errors.Is(ErrDuplicate, ErrConstraint), "ErrDuplicate should not match ErrConstraint")
}

func TestErrors_CanBeWrapped(t *testing.T) {
	wrapped := fmt.Errorf("content 123: %w", ErrNotFound)
	assert.True(t, errors.Is(wrapped, ErrNotFound), "wrapped error should match ErrNotFound")
}
