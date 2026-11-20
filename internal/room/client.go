package room

import (
	"sync"

	"github.com/gorilla/websocket"
)

// Client represents a connected WebSocket viewer or host.
type Client struct {
	conn   *websocket.Conn
	mu     sync.Mutex // guards concurrent writes to websocket connection
	IsHost bool
	closed bool
}

// NewClient initializes a new Client instance.
func NewClient(conn *websocket.Conn, isHost bool) *Client {
	return &Client{
		conn:   conn,
		IsHost: isHost,
	}
}

// Send writes a JSON message to the client's WebSocket connection.
func (c *Client) Send(msg Message) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.closed {
		return websocket.ErrCloseSent
	}
	return c.conn.WriteJSON(msg)
}

// Close safely shuts down the client connection.
func (c *Client) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.closed {
		return nil
	}
	c.closed = true
	return c.conn.Close()
}
