package room

import (
	"crypto/rand"
	"encoding/hex"
	"sync"
)

// Room represents a single watch party session.
type Room struct {
	ID        string         `json:"id"`
	HostToken string         `json:"hostToken"`
	PlayBack  *PlaybackState `json:"playback"`
	mu        sync.RWMutex
	clients   map[*Client]bool
}

// generateToken creates a random hex string of given byte length.
func generateToken(n int) string {
	bytes := make([]byte, n)
	_, _ = rand.Read(bytes)
	return hex.EncodeToString(bytes)
}

// NewRoom creates a new watch party room with randomized IDs.
func NewRoom() *Room {
	return &Room{
		ID:        generateToken(4),
		HostToken: generateToken(16),
		clients:   make(map[*Client]bool),
		PlayBack:  NewPlaybackState(),
	}
}

// ClientCount returns the number of connected clients.
func (r *Room) ClientCount() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.clients)
}

// Join registers a new client and broadcasts updated state.
func (r *Room) Join(c *Client) {
	r.mu.Lock()
	r.clients[c] = true
	count := len(r.clients)
	r.mu.Unlock()

	// 1. Send current state immediately to the joining viewer
	_ = c.Send(Message{
		Type:      "SYNC",
		Position:  r.PlayBack.CurrentPosition(),
		IsPlaying: r.PlayBack.Playing(),
		Count:     count,
	})

	// 2. Broadcast updated participant count to all clients
	r.Broadcast(Message{
		Type:  "JOIN",
		Count: count,
	})
}

// Leave removes a client from the room and announces the departure.
func (r *Room) Leave(c *Client) {
	r.mu.Lock()
	if !r.clients[c] {
		r.mu.Unlock()
		return
	}
	delete(r.clients, c)
	count := len(r.clients)
	r.mu.Unlock()

	_ = c.Close()

	r.Broadcast(Message{
		Type:  "LEAVE",
		Count: count,
	})
}

// Broadcast sends a message to all connected clients.
func (r *Room) Broadcast(msg Message) {
	r.mu.RLock()
	clientList := make([]*Client, 0, len(r.clients))
	for c := range r.clients {
		clientList = append(clientList, c)
	}
	r.mu.RUnlock()

	for _, c := range clientList {
		if err := c.Send(msg); err != nil {
			go r.Leave(c)
		}
	}
}
