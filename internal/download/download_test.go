package download

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
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
		assert.NotEmpty(t, s, "status constant is empty")
	}
}

func TestDownloadHasLastTransitionAt(t *testing.T) {
	now := time.Now()
	d := Download{
		ID:               1,
		Status:           StatusQueued,
		LastTransitionAt: now,
	}

	assert.False(t, d.LastTransitionAt.IsZero(), "LastTransitionAt should be set")
	assert.True(t, d.LastTransitionAt.Equal(now),
		"LastTransitionAt = %v, want %v", d.LastTransitionAt, now)
}
