package tvdb

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestTypes(t *testing.T) {
	// Verify types are usable
	s := Series{
		ID:       12345,
		Name:     "Breaking Bad",
		Year:     2008,
		Status:   "Ended",
		Overview: "A chemistry teacher becomes a drug lord.",
	}
	assert.Equal(t, 12345, s.ID)
	assert.Equal(t, "Breaking Bad", s.Name)

	e := Episode{
		ID:      1,
		Season:  1,
		Episode: 1,
		Name:    "Pilot",
		AirDate: time.Date(2008, 1, 20, 0, 0, 0, 0, time.UTC),
	}
	assert.Equal(t, 1, e.Season)
	assert.Equal(t, "Pilot", e.Name)
}
