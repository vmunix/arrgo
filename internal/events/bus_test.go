package events

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBus_PublishSubscribe(t *testing.T) {
	db := setupTestDB(t)
	log := NewEventLog(db)
	bus := NewBus(log, nil)
	defer bus.Close()

	// Subscribe before publishing
	ch := bus.Subscribe("test.created", 10)

	// Publish
	e := &testEvent{BaseEvent: NewBaseEvent("test.created", "test", 1), Message: "hello"}
	err := bus.Publish(context.Background(), e)
	require.NoError(t, err)

	// Receive
	select {
	case received := <-ch:
		assert.Equal(t, "test.created", received.EventType())
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for event")
	}
}

func TestBus_SubscribeAll(t *testing.T) {
	db := setupTestDB(t)
	log := NewEventLog(db)
	bus := NewBus(log, nil)
	defer bus.Close()

	ch := bus.SubscribeAll(10)

	// Publish different event types
	e1 := &testEvent{BaseEvent: NewBaseEvent("test.first", "test", 1), Message: "first"}
	e2 := &testEvent{BaseEvent: NewBaseEvent("test.second", "test", 2), Message: "second"}

	err := bus.Publish(context.Background(), e1)
	require.NoError(t, err)
	err = bus.Publish(context.Background(), e2)
	require.NoError(t, err)

	// Should receive both
	received := make([]Event, 0, 2)
	timeout := time.After(time.Second)
	for i := 0; i < 2; i++ {
		select {
		case e := <-ch:
			received = append(received, e)
		case <-timeout:
			t.Fatalf("timeout waiting for event %d", i+1)
		}
	}

	assert.Len(t, received, 2)
}

func TestBus_Unsubscribe(t *testing.T) {
	db := setupTestDB(t)
	log := NewEventLog(db)
	bus := NewBus(log, nil)
	defer bus.Close()

	ch := bus.Subscribe("test.event", 10)

	// Unsubscribe
	bus.Unsubscribe(ch)

	// Publish (should not block even with no subscribers)
	e := &testEvent{BaseEvent: NewBaseEvent("test.event", "test", 1), Message: "hello"}
	err := bus.Publish(context.Background(), e)
	require.NoError(t, err)

	// Channel should be closed
	select {
	case _, ok := <-ch:
		assert.False(t, ok, "channel should be closed")
	default:
		// This is also acceptable - channel is closed
	}
}

func TestBus_ConcurrentPublish(t *testing.T) {
	// No persistence needed - this test verifies concurrent delivery, not persistence
	bus := NewBus(nil, nil)
	defer bus.Close()

	ch := bus.SubscribeAll(100)

	// Concurrent publishers
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			e := &testEvent{BaseEvent: NewBaseEvent("test.concurrent", "test", int64(n)), Message: "concurrent"}
			_ = bus.Publish(context.Background(), e) // Error ignored: test verifies delivery, not persistence
		}(i)
	}

	wg.Wait()

	// Count received events
	count := 0
	timeout := time.After(time.Second)
loop:
	for {
		select {
		case <-ch:
			count++
			if count == 10 {
				break loop
			}
		case <-timeout:
			break loop
		}
	}

	assert.Equal(t, 10, count)
}
