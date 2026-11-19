package main

import (
	"context"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"
)

type MovieState struct {
	mu           sync.RWMutex
	filePath     string
	originalName string
}

func (m *MovieState) Set(path, name string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.filePath != "" && m.filePath != path {
		_ = os.Remove(m.filePath) // Delete old temp file
	}

	m.filePath = path
	m.originalName = name
}

func (m *MovieState) Get() (path, name string, ok bool) {
	m.mu.RLock()
	defer m.mu.Unlock()

	if m.filePath == "" {
		return "", "", false
	}

	return m.filePath, m.originalName, true
}

// 10GB
const maxUploadSize = 10 << 30

func uploadHandler(state *MovieState) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
			return
		}

		r.Body = http.MaxBytesReader(w, r.Body, maxUploadSize)

		file, header, err := r.FormFile("video")
		if err != nil {
			http.Error(w, "Error retrieving file: "+err.Error(), http.StatusBadRequest)
			return
		}

		defer file.Close()

		tempFile, err := os.CreateTemp("", "streamla-*.mp4")
		if err != nil {
			http.Error(w, "Error creating temp file", http.StatusInternalServerError)
			return
		}

		defer tempFile.Close()

		if _, err := io.Copy(tempFile, file); err != nil {
			_ = os.Remove(tempFile.Name())
			http.Error(w, "Error saving file : "+err.Error(), http.StatusInternalServerError)
			return
		}

		state.Set(tempFile.Name(), header.Filename)

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{
			"message":  "Uploaded successfully",
			"filename": header.Filename,
		})
	}
}

func streamHandler(state *MovieState) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		path, _, ok := state.Get()
		if !ok {
			http.Error(w, "No Movie currently selected", http.StatusNotFound)
			return
		}

		file, err := os.Open(path)
		if err != nil {
			http.Error(w, "Video not found", http.StatusNotFound)
			return
		}

		defer file.Close()

		stat, err := file.Stat()
		if err != nil {
			http.Error(w, "Could not read video metadata", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "video/mp4")
		http.ServeContent(w, r, stat.Name(), stat.ModTime(), file)
	}
}

// cleanup
func (m *MovieState) Cleanup() {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.filePath != "" {
		_ = os.Remove(m.filePath)

		m.filePath = ""
		m.originalName = ""

		log.Println("Temporary movie file cleaned up")
	}
}

func main() {
	// http.HandleFunc("/stream",streamHandler("song.mp3"))
	moviestate := &MovieState{}
	room := NewRoom()

	log.Printf("Room created..ID : %s (Host Token: %s)", room.ID, room.HostToken)

	mux := http.NewServeMux()
	mux.HandleFunc("/stream", streamHandler(moviestate))
	mux.HandleFunc("/api/upload", uploadHandler(moviestate))
	mux.HandleFunc("/ws", wsHandler(room))

	server := &http.Server{
		Addr:    ":8080",
		Handler: mux,
	}

	go func() {
		log.Println("Server running on http://localhost:8080")
		// err server closed prevents treating normal shutdown as a crash
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Server error: %v", err)
		}
	}()

	// ctrl + c(sigint) or sigterm
	stopChan := make(chan os.Signal, 1)
	signal.Notify(stopChan, os.Interrupt, syscall.SIGTERM)
	<-stopChan

	log.Println("Shutting down server...")

	// grace period of 5 seconds to finish in-flight requests
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_ = server.Shutdown(ctx)
	moviestate.Cleanup()
	log.Println("Server exited successfully")
}
