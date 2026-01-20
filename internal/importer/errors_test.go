// internal/importer/errors_test.go
package importer

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestErrors(t *testing.T) {
	// Verify errors are distinct
	assert.False(t, errors.Is(ErrDownloadNotFound, ErrDownloadNotReady), "errors should be distinct")
	assert.False(t, errors.Is(ErrNoVideoFile, ErrCopyFailed), "errors should be distinct")

	// Verify all errors have messages
	errs := []error{
		ErrDownloadNotFound,
		ErrDownloadNotReady,
		ErrNoVideoFile,
		ErrCopyFailed,
		ErrDestinationExists,
		ErrPathTraversal,
	}
	for _, err := range errs {
		assert.NotEmpty(t, err.Error(), "error %v should have a message", err)
	}
}
