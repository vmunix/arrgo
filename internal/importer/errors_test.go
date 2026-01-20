// internal/importer/errors_test.go
package importer

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestErrors(t *testing.T) {
	// Verify errors are distinct
	require.NotErrorIs(t, ErrDownloadNotFound, ErrDownloadNotReady, "errors should be distinct")
	require.NotErrorIs(t, ErrNoVideoFile, ErrCopyFailed, "errors should be distinct")

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
