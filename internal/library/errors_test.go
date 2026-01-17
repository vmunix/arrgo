package library

import (
	"errors"
	"fmt"
	"testing"
)

func TestErrors_AreDistinct(t *testing.T) {
	if errors.Is(ErrNotFound, ErrDuplicate) {
		t.Error("ErrNotFound should not match ErrDuplicate")
	}
	if errors.Is(ErrNotFound, ErrConstraint) {
		t.Error("ErrNotFound should not match ErrConstraint")
	}
	if errors.Is(ErrDuplicate, ErrConstraint) {
		t.Error("ErrDuplicate should not match ErrConstraint")
	}
}

func TestErrors_CanBeWrapped(t *testing.T) {
	wrapped := fmt.Errorf("content 123: %w", ErrNotFound)
	if !errors.Is(wrapped, ErrNotFound) {
		t.Error("wrapped error should match ErrNotFound")
	}
}
