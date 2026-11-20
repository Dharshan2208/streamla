package media

import (
	"log"
	"os"
	"sync"
)

// MovieState safely manages the active movie file on disk.
type MovieState struct {
	mu           sync.RWMutex
	filePath     string
	originalName string
	isTemp       bool
}

// NewMovieState initializes a MovieState instance.
func NewMovieState() *MovieState {
	return &MovieState{}
}

// Set updates the active movie file. If the previous file was a temporary upload,
// it deletes it to conserve disk space.
func (m *MovieState) Set(path, name string, isTemp bool) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.isTemp && m.filePath != "" && m.filePath != path {
		_ = os.Remove(m.filePath)
	}

	m.filePath = path
	m.originalName = name
	m.isTemp = isTemp
}

// Get returns the current active movie path, original filename, and whether a movie is loaded.
func (m *MovieState) Get() (path, name string, ok bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.filePath == "" {
		return "", "", false
	}

	return m.filePath, m.originalName, true
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
		log.Println("Temporary movie file cleaned up")
	}
}
