package media

import (
	"log"
	"os"
	"sync"
)

// MovieState safely manages the active movie file and its probed metadata on disk.
type MovieState struct {
	mu           sync.RWMutex
	filePath     string
	originalName string
	isTemp       bool
	probe        *MediaProbeResult
}

// NewMovieState initializes a MovieState instance.
func NewMovieState() *MovieState {
	return &MovieState{}
}

// Set updates the active movie file and probes its stream metadata.
func (m *MovieState) Set(path, name string, isTemp bool) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.isTemp && m.filePath != "" && m.filePath != path {
		_ = os.Remove(m.filePath)
	}

	m.filePath = path
	m.originalName = name
	m.isTemp = isTemp

	// Probe stream codecs, audio tracks, and subtitles
	if probe, err := ProbeMedia(path); err == nil {
		m.probe = probe
	} else {
		log.Printf("Warning: ffprobe failed for %q: %v", path, err)
		m.probe = &MediaProbeResult{
			FormatName: "unknown",
			Audio:      []StreamInfo{},
			Subtitles:  []StreamInfo{},
		}
	}
}

// Get returns the current active movie path, original filename, probe metadata, and whether loaded.
func (m *MovieState) Get() (path, name string, probe *MediaProbeResult, ok bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.filePath == "" {
		return "", "", nil, false
	}

	return m.filePath, m.originalName, m.probe, true
}

// Cleanup removes any temporary uploaded file on server shutdown.
func (m *MovieState) Cleanup() {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.isTemp && m.filePath != "" {
		_ = os.Remove(m.filePath)
		m.filePath = ""
		m.originalName = ""
		m.isTemp = false
		m.probe = nil
		log.Println("Temporary movie file cleaned up")
	}
}
