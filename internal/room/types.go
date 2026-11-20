package room

// Message represents signaling and sync messages between server and clients.
type Message struct {
	Type       string  `json:"type"`                 // PLAY, PAUSE, SEEK, PREPARE_PLAY, PREPARE_SEEK, PING, PONG, HEARTBEAT, SYNC, JOIN, LEAVE, MOVIE_CHANGED
	Position   float64 `json:"position"`             // Timeline position in seconds
	IsPlaying  bool    `json:"isPlaying"`            // Playback status
	Count      int     `json:"count"`                // Current connected participant count
	Token      string  `json:"token,omitempty"`      // Secret token for host authentication
	T1         float64 `json:"t1,omitempty"`         // Client send timestamp in ms
	T2         float64 `json:"t2,omitempty"`         // Server receive timestamp in ms
	T3         float64 `json:"t3,omitempty"`         // Server transmit timestamp in ms
	ResumeAt   float64 `json:"resumeAt,omitempty"`   // Scheduled resume timestamp in server ms
	ServerTime float64 `json:"serverTime,omitempty"` // Server current timestamp in ms
}
