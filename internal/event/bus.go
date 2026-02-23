package event

import (
	"sync"

	"github.com/google/uuid"
)

type InMemoryBus struct {
	mu          sync.RWMutex
	subscribers map[string]chan Event
}

func NewBus() *InMemoryBus {
	return &InMemoryBus{
		subscribers: make(map[string]chan Event),
	}
}

func (b *InMemoryBus) Publish(e Event) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	for _, ch := range b.subscribers {
		// Non-blocking send to avoid blocking the publisher if a subscriber is slow
		select {
		case ch <- e:
		default:
			// If channel is full, we drop the message for this subscriber
			// Ideally we would log this
		}
	}
}

func (b *InMemoryBus) Subscribe() (<-chan Event, func()) {
	b.mu.Lock()
	defer b.mu.Unlock()

	id := uuid.NewString()
	ch := make(chan Event, 100) // Buffer to handle bursts
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
