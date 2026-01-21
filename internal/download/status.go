package download

import "time"

// TransitionEvent is emitted when a download changes state.
type TransitionEvent struct {
	DownloadID int64
	From       Status
	To         Status
	At         time.Time
}

// TransitionHandler processes state transition events.
type TransitionHandler func(event TransitionEvent)

// validTransitions defines allowed state transitions.
// Key is the "from" status, value is list of valid "to" statuses.
var validTransitions = map[Status][]Status{
	StatusQueued:      {StatusDownloading, StatusCompleted, StatusFailed}, // completed: can skip downloading if fast
	StatusDownloading: {StatusCompleted, StatusFailed},
	StatusCompleted:   {StatusImporting, StatusSkipped, StatusFailed}, // skipped: duplicate detected
	StatusImporting:   {StatusImported, StatusFailed},
	StatusImported:    {StatusCleaned, StatusFailed},
	StatusCleaned:     {},             // terminal - no transitions out
	StatusSkipped:     {},             // terminal - duplicate was detected
	StatusFailed:      {StatusQueued}, // allow retry
}

// CanTransitionTo returns true if transitioning from s to target is valid.
func (s Status) CanTransitionTo(target Status) bool {
	valid, ok := validTransitions[s]
	if !ok {
		return false
	}
	for _, v := range valid {
		if v == target {
			return true
		}
	}
	return false
}

// IsTerminal returns true if this status has no valid outgoing transitions
// (except failed which can retry).
func (s Status) IsTerminal() bool {
	return s == StatusCleaned || s == StatusFailed || s == StatusSkipped
}
