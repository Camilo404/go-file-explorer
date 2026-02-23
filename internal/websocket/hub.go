package websocket

import (
	"encoding/json"
	"log/slog"

	"go-file-explorer/internal/event"
)

type Hub struct {
	// Registered clients.
	clients map[*Client]bool

	// Register requests from the clients.
	register chan *Client

	// Unregister requests from clients.
	unregister chan *Client

	// Event bus to listen for events
	bus event.Bus
}

func NewHub(bus event.Bus) *Hub {
	return &Hub{
		register:   make(chan *Client),
		unregister: make(chan *Client),
		clients:    make(map[*Client]bool),
		bus:        bus,
	}
}

func (h *Hub) Run() {
	// Subscribe to event bus
	events, unsubscribe := h.bus.Subscribe()
	defer unsubscribe()

	for {
		select {
		case client := <-h.register:
			h.clients[client] = true
		case client := <-h.unregister:
			if _, ok := h.clients[client]; ok {
				delete(h.clients, client)
				close(client.send)
			}
		case e := <-events:
			// Marshal event to JSON
			message, err := json.Marshal(e)
			if err != nil {
				slog.Error("failed to marshal event", "error", err)
				continue
			}
			// Broadcast to all clients
			for client := range h.clients {
				select {
				case client.send <- message:
				default:
					close(client.send)
					delete(h.clients, client)
				}
			}
		}
	}
}
