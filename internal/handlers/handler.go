// internal/handlers/handler.go
package handlers

import (
	"context"
	"log/slog"

	"github.com/vmunix/arrgo/internal/events"
)

// Handler processes events of specific types.
type Handler interface {
	// Start begins processing events (blocking).
	Start(ctx context.Context) error

	// Name returns handler name for logging.
	Name() string
}

// BaseHandler provides common handler functionality.
type BaseHandler struct {
	bus    *events.Bus
	logger *slog.Logger
}

// NewBaseHandler creates a base handler.
func NewBaseHandler(bus *events.Bus, logger *slog.Logger) *BaseHandler {
	if logger == nil {
		logger = slog.Default()
	}
	return &BaseHandler{
		bus:    bus,
		logger: logger,
	}
}

// Bus returns the event bus.
func (h *BaseHandler) Bus() *events.Bus {
	return h.bus
}

// Logger returns the handler's logger.
func (h *BaseHandler) Logger() *slog.Logger {
	return h.logger
}
