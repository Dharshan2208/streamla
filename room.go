package main

import (
	"crypto/rand"
	"encoding/hex"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

type Message struct {
	Type      string  `json:"type"` // PLAY,SEEK,PAUSE,SYNC
	Position  float64 `json:"position"`
	IsPlaying bool    `json:"isPlaying"`
	Count     int     `json:"count"`           // connected viewers count
	Token     string  `json:"token,omitempty"` // host secret token for auth
}

type Client struct {
	conn   *websocket.Conn
	mu     sync.Mutex // protects concurrent writes to webscoket so
	IsHost bool
}

type Room struct {
	ID        string         `json:"id"`
	HostToken string         `json:"hostToken"`
	PlayBack  *PlaybackState `json:"playback"`
	mu        sync.RWMutex
	clients   map[*Client]bool
}

func (c *Client) Send(msg Message) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.conn.WriteJSON(msg)
}

// generateToken create a random hex string of given byte length
func generateToken(n int) string {
	bytes := make([]byte, n)
	_, _ = rand.Read(bytes)
	return hex.EncodeToString(bytes)
}

func NewRoom() *Room {
	return &Room{
		ID:        generateToken(4),
		HostToken: generateToken(16),
		clients:   make(map[*Client]bool),
		PlayBack: &PlaybackState{
			Position:  0,
			IsPlaying: false,
			UpdatedAt: time.Now(),
		},
	}
}

type PlaybackState struct {
	mu        sync.RWMutex
	Position  float64   `json:"Position"`
	IsPlaying bool      `json""isPlaying`
	UpdatedAt time.Time `json:"updatedAt"`
}

// current position calculates the real time position based on elapsed wall clock time
func (p *PlaybackState) CurrentPosition() float64 {
	p.mu.RLock()
	defer p.mu.RUnlock()

	if !p.IsPlaying {
		return p.Position
	}

	return p.Position + time.Since(p.UpdatedAt).Seconds()
}

// update changes the state when the host sends PLAY,PAUSE or SEEK
func (p *PlaybackState) Update(pos float64, isPlaying bool) {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.Position = pos
	p.IsPlaying = isPlaying
	p.UpdatedAt = time.Now()
}
