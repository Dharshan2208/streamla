package main

import (
	"crypto/rand"
	"encoding/hex"
	"net/http"
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
	IsPlaying bool      `json:"isPlaying"`
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

// functions for join leave and broadcast
func (r *Room) Join(c *Client) {
	r.mu.Lock()
	r.clients[c] = true
	count := len(r.clients)
	r.mu.Unlock()

	// 1.sending the current state immediately to this new viewer
	_ = c.Send(Message{
		Type:      "SYNC",
		Position:  r.PlayBack.CurrentPosition(),
		IsPlaying: r.PlayBack.IsPlaying,
		Count:     count,
	})

	// 2.announcing evreyong the updated participant count
	r.Broadcast(Message{
		Type:  "JOIN",
		Count: count,
	})
}

func (r *Room) Leave(c *Client) {
	r.mu.Lock()
	if !r.clients[c] {
		r.mu.Unlock()
		return
	}

	delete(r.clients, c)
	count := len(r.clients)
	r.mu.Unlock()

	_ = c.conn.Close()

	r.Broadcast(Message{
		Type:  "LEAVE",
		Count: count,
	})
}

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

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

func (r *Room) ClientCount() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.clients)
}

func wsHandler(room *Room) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}

		token := r.URL.Query().Get("token")

		isHost := (token != "" && token == room.HostToken)

		client := &Client{
			conn:   conn,
			IsHost: isHost,
		}

		room.Join(client)
		defer room.Leave(client)

		for {
			var msg Message
			if err := conn.ReadJSON(&msg); err != nil {
				// client disconnected or closed tab
				break
			}

			// secutiy : only the host may control the playback
			if msg.Type == "PLAY" || msg.Type == "PAUSE" || msg.Type == "SEEK" {
				if !client.IsHost {
					continue
				}
			}

			isPlaying := (msg.Type == "PLAY")
			switch msg.Type {
			case "PAUSE":
				isPlaying = false
			case "SEEK":
				isPlaying = room.PlayBack.IsPlaying
			}

			room.PlayBack.Update(msg.Position, isPlaying)

			room.Broadcast(Message{
				Type:      msg.Type,
				Position:  msg.Position,
				IsPlaying: isPlaying,
				Count:     room.ClientCount(),
			})
		}
	}
}
