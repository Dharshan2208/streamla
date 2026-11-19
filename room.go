package main

import (
	"crypto/rand"
	"encoding/hex"
	"sync"
	"time"
)

type Room struct {
	ID        string         `json:"id"`
	HostToken string         `json:"hostToken"`
	PlayBack  *PlaybackState `json:"playback"`
}

// generateToken create a random hex string of given byte length
func generateToken(n int) string {
	bytes := make([]byte, n)
	_, _ = rand.Read(bytes)
	return hex.EncodeToString(bytes)
}

func NewRoom() *Room{
	return &Room{
		ID : generateToken(4),
		HostToken: generateToken(16),
		PlayBack: &PlaybackState{
			Position: 0,
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
