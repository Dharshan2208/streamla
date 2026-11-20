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

	now := time.Now()
	// If playback is scheduled to resume in the future, return base position
	if now.Before(p.UpdatedAt) {
		return p.Position
	}

	return p.Position + now.Sub(p.UpdatedAt).Seconds()
}

// Playing returns true if the media is currently playing or scheduled to play.
func (p *PlaybackState) Playing() bool {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.IsPlaying
}

// Update changes the state immediately (e.g. on direct seek or initial upload).
func (p *PlaybackState) Update(pos float64, isPlaying bool) {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.Position = pos
	p.IsPlaying = isPlaying
	p.UpdatedAt = time.Now()
}

// SchedulePlay sets the playback to resume at a specified future server time.
func (p *PlaybackState) SchedulePlay(pos float64, resumeAt time.Time) {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.Position = pos
	p.IsPlaying = true
	p.UpdatedAt = resumeAt
}

// Pause pauses playback immediately at the given position.
func (p *PlaybackState) Pause(pos float64) {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.Position = pos
	p.IsPlaying = false
	p.UpdatedAt = time.Now()
}
