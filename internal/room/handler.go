package room

import (
	"net/http"

	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

// WSHandler creates an HTTP handler for WebSocket connections.
func WSHandler(room *Room) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}

		token := r.URL.Query().Get("token")
		isHost := (token != "" && token == room.HostToken)

		client := NewClient(conn, isHost)

		room.Join(client)
		defer room.Leave(client)

		for {
			var msg Message
			if err := conn.ReadJSON(&msg); err != nil {
				// Client disconnected or closed connection
				break
			}

			switch msg.Type {
			case "PLAY", "PAUSE", "SEEK":
				if !client.IsHost {
					continue // Viewers cannot control playback
				}

				isPlaying := (msg.Type == "PLAY")
				if msg.Type == "PAUSE" {
					isPlaying = false
				} else if msg.Type == "SEEK" {
					isPlaying = room.PlayBack.Playing()
				}

				room.PlayBack.Update(msg.Position, isPlaying)

				room.Broadcast(Message{
					Type:      msg.Type,
					Position:  msg.Position,
					IsPlaying: isPlaying,
					Count:     room.ClientCount(),
				})

			case "SYNC":
				_ = client.Send(Message{
					Type:      "SYNC",
					Position:  room.PlayBack.CurrentPosition(),
					IsPlaying: room.PlayBack.Playing(),
					Count:     room.ClientCount(),
				})
			}
		}
	}
}
