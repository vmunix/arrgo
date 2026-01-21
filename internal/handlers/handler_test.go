// internal/handlers/handler_test.go
package handlers

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vmunix/arrgo/internal/events"
)

// mockHandler is a test implementation of Handler
type mockHandler struct {
	name    string
	started bool
	stopped bool
}

func (h *mockHandler) Name() string { return h.name }

func (h *mockHandler) Start(ctx context.Context) error {
	h.started = true
	<-ctx.Done()
	h.stopped = true
	return ctx.Err()
}

func TestHandler_StartStop(t *testing.T) {
	h := &mockHandler{name: "test"}

	ctx, cancel := context.WithCancel(context.Background())

	// Start in background
	done := make(chan error, 1)
	go func() {
		done <- h.Start(ctx)
	}()

	// Wait for start
	time.Sleep(10 * time.Millisecond)
	assert.True(t, h.started)
	assert.False(t, h.stopped)

	// Stop
	cancel()
	err := <-done
	require.ErrorIs(t, err, context.Canceled)
	assert.True(t, h.stopped)
}

func TestBaseHandler_Fields(t *testing.T) {
	bus := events.NewBus(nil, nil)
	defer bus.Close()

	base := NewBaseHandler(bus, nil)
	assert.NotNil(t, base.Bus())
	assert.NotNil(t, base.Logger())
}
