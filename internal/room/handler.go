package room

import (
	"net/http"
	"time"

	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

// ScheduledResumeDelay is the latency buffer allocated for clients to buffer and resume simultaneously.
const ScheduledResumeDelay = 350 * time.Millisecond

// WSHandler creates an HTTP handler for WebSocket signaling and timeline synchronization.
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
				// Client disconnected or tab closed
				break
			}

			switch msg.Type {
			case "PING":
				nowMs := float64(time.Now().UnixNano()) / 1e6
				_ = client.Send(Message{
					Type: "PONG",
					T1:   msg.T1,
					T2:   nowMs,
					T3:   nowMs,
				})

			case "PLAY":
				if !client.IsHost {
					continue
				}
				resumeAt := time.Now().Add(ScheduledResumeDelay)
				room.PlayBack.SchedulePlay(msg.Position, resumeAt)

				room.Broadcast(Message{
					Type:       "PREPARE_PLAY",
					Position:   msg.Position,
					IsPlaying:  true,
					ResumeAt:   float64(resumeAt.UnixMilli()),
					ServerTime: float64(time.Now().UnixMilli()),
					Count:      room.ClientCount(),
				})

			case "PAUSE":
				if !client.IsHost {
					continue
				}
				room.PlayBack.Pause(msg.Position)

				room.Broadcast(Message{
					Type:       "PAUSE",
					Position:   msg.Position,
					IsPlaying:  false,
					ServerTime: float64(time.Now().UnixMilli()),
					Count:      room.ClientCount(),
				})

			case "SEEK":
				if !client.IsHost {
					continue
				}
				if room.PlayBack.Playing() {
					resumeAt := time.Now().Add(ScheduledResumeDelay)
					room.PlayBack.SchedulePlay(msg.Position, resumeAt)

					room.Broadcast(Message{
						Type:       "PREPARE_SEEK",
						Position:   msg.Position,
						IsPlaying:  true,
						ResumeAt:   float64(resumeAt.UnixMilli()),
						ServerTime: float64(time.Now().UnixMilli()),
						Count:      room.ClientCount(),
					})
				} else {
					room.PlayBack.Pause(msg.Position)

					room.Broadcast(Message{
						Type:       "SEEK",
						Position:   msg.Position,
						IsPlaying:  false,
						ServerTime: float64(time.Now().UnixMilli()),
						Count:      room.ClientCount(),
					})
				}

			case "SYNC":
				_ = client.Send(Message{
					Type:       "SYNC",
					Position:   room.PlayBack.CurrentPosition(),
					IsPlaying:  room.PlayBack.Playing(),
					ServerTime: float64(time.Now().UnixMilli()),
					Count:      room.ClientCount(),
				})
			}
		}
	}
}
