package download

import "testing"

func TestStatusConstants(t *testing.T) {
	// Verify all expected statuses exist
	statuses := []Status{
		StatusQueued,
		StatusDownloading,
		StatusCompleted,
		StatusFailed,
		StatusImported,
		StatusCleaned,
	}

	for _, s := range statuses {
		if s == "" {
			t.Error("status constant is empty")
		}
	}
}
