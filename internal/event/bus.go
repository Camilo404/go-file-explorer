package event

import (
	"log/slog"
	"sync"
	"sync/atomic"

	"github.com/google/uuid"
)

type InMemoryBus struct {
	mu            sync.RWMutex
	subscribers   map[string]chan Event
	droppedEvents atomic.Uint64
}

func NewBus() *InMemoryBus {
	return &InMemoryBus{
		subscribers: make(map[string]chan Event),
	}
}

func (b *InMemoryBus) Publish(e Event) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	for id, ch := range b.subscribers {
		// Non-blocking send to avoid blocking the publisher if a subscriber is slow
		select {
		case ch <- e:
		default:
			// If channel is full, we drop the message for this subscriber
			b.droppedEvents.Add(1)
			slog.Warn("event bus: subscriber channel full, dropping event",
				"subscriber_id", id,
				"event_type", e.Type,
				"event_id", e.ID,
				"total_dropped", b.droppedEvents.Load())
		}
	}
}

func (b *InMemoryBus) Subscribe() (<-chan Event, func()) {
	b.mu.Lock()
	defer b.mu.Unlock()

	id := uuid.NewString()
	// Increased buffer size to handle bursts better
	ch := make(chan Event, 1000)
	b.subscribers[id] = ch

	unsubscribe := func() {
		b.mu.Lock()
		defer b.mu.Unlock()
		if ch, exists := b.subscribers[id]; exists {
			close(ch)
			delete(b.subscribers, id)
		}
	}

	return ch, unsubscribe
}

func (b *InMemoryBus) DroppedCount() uint64 {
	return b.droppedEvents.Load()
}
