// internal/importer/errors_test.go
package importer

import (
	"errors"
	"testing"
)

func TestErrors(t *testing.T) {
	// Verify errors are distinct
	if errors.Is(ErrDownloadNotFound, ErrDownloadNotReady) {
		t.Error("errors should be distinct")
	}
	if errors.Is(ErrNoVideoFile, ErrCopyFailed) {
		t.Error("errors should be distinct")
	}

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
		if err.Error() == "" {
			t.Errorf("error %v should have a message", err)
		}
	}
}
