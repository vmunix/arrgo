package library

import "errors"

var (
	// ErrNotFound indicates the requested entity doesn't exist.
	ErrNotFound = errors.New("not found")

	// ErrDuplicate indicates a unique constraint violation.
	ErrDuplicate = errors.New("duplicate entry")

	// ErrConstraint indicates a foreign key or check constraint violation.
	ErrConstraint = errors.New("constraint violation")
)
