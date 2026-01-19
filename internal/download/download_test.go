package download

import (
	"testing"
	"time"
)

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

func TestDownloadHasLastTransitionAt(t *testing.T) {
	now := time.Now()
	d := Download{
		ID:               1,
		Status:           StatusQueued,
		LastTransitionAt: now,
	}

	if d.LastTransitionAt.IsZero() {
		t.Error("LastTransitionAt should be set")
	}
	if !d.LastTransitionAt.Equal(now) {
		t.Errorf("LastTransitionAt = %v, want %v", d.LastTransitionAt, now)
	}
}
