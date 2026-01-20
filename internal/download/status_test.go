package download

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestCanTransitionTo_ValidTransitions(t *testing.T) {
	tests := []struct {
		from Status
		to   Status
	}{
		{StatusQueued, StatusDownloading},
		{StatusQueued, StatusFailed},
		{StatusDownloading, StatusCompleted},
		{StatusDownloading, StatusFailed},
		{StatusCompleted, StatusImported},
		{StatusCompleted, StatusFailed},
		{StatusImported, StatusCleaned},
		{StatusImported, StatusFailed},
		{StatusFailed, StatusQueued}, // retry
	}

	for _, tt := range tests {
		t.Run(string(tt.from)+"->"+string(tt.to), func(t *testing.T) {
			assert.True(t, tt.from.CanTransitionTo(tt.to),
				"%s should be able to transition to %s", tt.from, tt.to)
		})
	}
}

func TestCanTransitionTo_InvalidTransitions(t *testing.T) {
	tests := []struct {
		from Status
		to   Status
	}{
		{StatusQueued, StatusCompleted},     // skip downloading
		{StatusQueued, StatusImported},      // skip multiple
		{StatusQueued, StatusCleaned},       // skip multiple
		{StatusDownloading, StatusQueued},   // backwards
		{StatusDownloading, StatusImported}, // skip completed
		{StatusCompleted, StatusQueued},     // backwards
		{StatusCompleted, StatusCleaned},    // skip imported
		{StatusImported, StatusQueued},      // backwards
		{StatusImported, StatusCompleted},   // backwards
		{StatusCleaned, StatusQueued},       // terminal
		{StatusCleaned, StatusFailed},       // terminal
	}

	for _, tt := range tests {
		t.Run(string(tt.from)+"->"+string(tt.to), func(t *testing.T) {
			assert.False(t, tt.from.CanTransitionTo(tt.to),
				"%s should NOT be able to transition to %s", tt.from, tt.to)
		})
	}
}

func TestIsTerminal(t *testing.T) {
	terminal := []Status{StatusCleaned, StatusFailed}
	nonTerminal := []Status{StatusQueued, StatusDownloading, StatusCompleted, StatusImported}

	for _, s := range terminal {
		assert.True(t, s.IsTerminal(), "%s should be terminal", s)
	}

	for _, s := range nonTerminal {
		assert.False(t, s.IsTerminal(), "%s should NOT be terminal", s)
	}
}

func TestTransitionEvent(t *testing.T) {
	event := TransitionEvent{
		DownloadID: 42,
		From:       StatusQueued,
		To:         StatusDownloading,
		At:         time.Now(),
	}

	assert.Equal(t, int64(42), event.DownloadID, "DownloadID not set")
	assert.Equal(t, StatusQueued, event.From, "From not set")
	assert.Equal(t, StatusDownloading, event.To, "To not set")
}
