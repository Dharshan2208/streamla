package room

import (
	"sync"
	"time"
)

// PlaybackState tracks the authoritative movie timeline.
type PlaybackState struct {
	mu        sync.RWMutex
	Position  float64   `json:"position"`
	IsPlaying bool      `json:"isPlaying"`
	UpdatedAt time.Time `json:"updatedAt"`
}

// NewPlaybackState creates a new playback state at position 0, paused.
func NewPlaybackState() *PlaybackState {
	return &PlaybackState{
		Position:  0,
		IsPlaying: false,
		UpdatedAt: time.Now(),
	}
}

// CurrentPosition calculates the real-time position based on elapsed wall-clock time.
func (p *PlaybackState) CurrentPosition() float64 {
	p.mu.RLock()
	defer p.mu.RUnlock()

	if !p.IsPlaying {
		return p.Position
	}

	return p.Position + time.Since(p.UpdatedAt).Seconds()
}

// Playing returns true if the media is currently playing.
func (p *PlaybackState) Playing() bool {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.IsPlaying
}

// Update changes the state when playback control events occur (PLAY, PAUSE, SEEK).
func (p *PlaybackState) Update(pos float64, isPlaying bool) {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.Position = pos
	p.IsPlaying = isPlaying
	p.UpdatedAt = time.Now()
}
