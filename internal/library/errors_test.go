package library

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestErrors_AreDistinct(t *testing.T) {
	require.NotErrorIs(t, ErrNotFound, ErrDuplicate, "ErrNotFound should not match ErrDuplicate")
	require.NotErrorIs(t, ErrNotFound, ErrConstraint, "ErrNotFound should not match ErrConstraint")
	require.NotErrorIs(t, ErrDuplicate, ErrConstraint, "ErrDuplicate should not match ErrConstraint")
}

func TestErrors_CanBeWrapped(t *testing.T) {
	wrapped := fmt.Errorf("content 123: %w", ErrNotFound)
	require.ErrorIs(t, wrapped, ErrNotFound, "wrapped error should match ErrNotFound")
}
