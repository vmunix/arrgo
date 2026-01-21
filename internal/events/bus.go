package events

import (
	"context"
	"log/slog"
	"sync"
)

// Bus is the central event bus for pub/sub.
type Bus struct {
	mu          sync.RWMutex
	subscribers map[string][]chan Event // eventType -> channels
	allSubs     []chan Event            // subscribers to all events
	log         *EventLog               // SQLite persistence (may be nil)
	logger      *slog.Logger
	closed      bool
}

// NewBus creates a new event bus.
// The EventLog is optional - pass nil to disable persistence.
func NewBus(log *EventLog, logger *slog.Logger) *Bus {
	if logger == nil {
		logger = slog.Default()
	}
	return &Bus{
		subscribers: make(map[string][]chan Event),
		log:         log,
		logger:      logger,
	}
}

// Publish sends an event to all subscribers and optionally persists it.
func (b *Bus) Publish(ctx context.Context, e Event) error {
	b.mu.RLock()
	if b.closed {
		b.mu.RUnlock()
		return nil
	}

	// Get subscribers for this event type
	subs := make([]chan Event, len(b.subscribers[e.EventType()]))
	copy(subs, b.subscribers[e.EventType()])

	// Get all-event subscribers
	allSubs := make([]chan Event, len(b.allSubs))
	copy(allSubs, b.allSubs)
	b.mu.RUnlock()

	// Persist event
	if b.log != nil {
		if _, err := b.log.Append(e); err != nil {
			b.logger.Error("failed to persist event", "type", e.EventType(), "error", err)
			// Continue - event delivery is more important than persistence
		}
	}

	// Deliver to type-specific subscribers (non-blocking)
	for _, ch := range subs {
		select {
		case ch <- e:
		default:
			b.logger.Warn("subscriber channel full, dropping event",
				"type", e.EventType(),
				"entity_type", e.EntityType(),
				"entity_id", e.EntityID())
		}
	}

	// Deliver to all-event subscribers (non-blocking)
	for _, ch := range allSubs {
		select {
		case ch <- e:
		default:
			b.logger.Warn("all-subscriber channel full, dropping event",
				"type", e.EventType())
		}
	}

	return nil
}

// Subscribe returns a channel for events of a specific type.
func (b *Bus) Subscribe(eventType string, bufferSize int) <-chan Event {
	b.mu.Lock()
	defer b.mu.Unlock()

	ch := make(chan Event, bufferSize)
	b.subscribers[eventType] = append(b.subscribers[eventType], ch)
	return ch
}

// SubscribeAll returns a channel for all events.
func (b *Bus) SubscribeAll(bufferSize int) <-chan Event {
	b.mu.Lock()
	defer b.mu.Unlock()

	ch := make(chan Event, bufferSize)
	b.allSubs = append(b.allSubs, ch)
	return ch
}

// SubscribeEntity returns events for a specific entity.
// Note: This is implemented by subscribing to all events and filtering.
// For high-volume scenarios, consider a more efficient approach.
func (b *Bus) SubscribeEntity(entityType string, entityID int64, bufferSize int) <-chan Event {
	allCh := b.SubscribeAll(bufferSize * 10) // larger buffer for filtering
	filtered := make(chan Event, bufferSize)

	go func() {
		for e := range allCh {
			if e.EntityType() == entityType && e.EntityID() == entityID {
				select {
				case filtered <- e:
				default:
					// Drop if full
				}
			}
		}
		close(filtered)
	}()

	return filtered
}

// Unsubscribe removes a subscription channel.
func (b *Bus) Unsubscribe(ch <-chan Event) {
	b.mu.Lock()
	defer b.mu.Unlock()

	// Remove from type-specific subscribers
	for eventType, subs := range b.subscribers {
		for i, sub := range subs {
			if sub == ch {
				b.subscribers[eventType] = append(subs[:i], subs[i+1:]...)
				close(sub)
				return
			}
		}
	}

	// Remove from all-event subscribers
	for i, sub := range b.allSubs {
		if sub == ch {
			b.allSubs = append(b.allSubs[:i], b.allSubs[i+1:]...)
			close(sub)
			return
		}
	}
}

// Close shuts down the bus and closes all subscriber channels.
func (b *Bus) Close() error {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.closed {
		return nil
	}
	b.closed = true

	// Close all type-specific subscriber channels
	for _, subs := range b.subscribers {
		for _, ch := range subs {
			close(ch)
		}
	}
	b.subscribers = nil

	// Close all-event subscriber channels
	for _, ch := range b.allSubs {
		close(ch)
	}
	b.allSubs = nil

	return nil
}
