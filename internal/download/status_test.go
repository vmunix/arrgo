package download

import "testing"

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
			if !tt.from.CanTransitionTo(tt.to) {
				t.Errorf("%s should be able to transition to %s", tt.from, tt.to)
			}
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
			if tt.from.CanTransitionTo(tt.to) {
				t.Errorf("%s should NOT be able to transition to %s", tt.from, tt.to)
			}
		})
	}
}

func TestIsTerminal(t *testing.T) {
	terminal := []Status{StatusCleaned, StatusFailed}
	nonTerminal := []Status{StatusQueued, StatusDownloading, StatusCompleted, StatusImported}

	for _, s := range terminal {
		if !s.IsTerminal() {
			t.Errorf("%s should be terminal", s)
		}
	}

	for _, s := range nonTerminal {
		if s.IsTerminal() {
			t.Errorf("%s should NOT be terminal", s)
		}
	}
}
