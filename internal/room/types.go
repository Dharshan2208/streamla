package room

// Message represents signaling messages between server and clients.
type Message struct {
	Type      string  `json:"type"`            // PLAY, PAUSE, SEEK, SYNC, JOIN, LEAVE, MOVIE_CHANGED
	Position  float64 `json:"position"`        // Timeline position in seconds
	IsPlaying bool    `json:"isPlaying"`       // Playback status
	Count     int     `json:"count"`           // Current connected participant count
	Token     string  `json:"token,omitempty"` // Secret token for host authentication
}
