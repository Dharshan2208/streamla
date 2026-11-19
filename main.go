package main

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
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
	defer m.mu.RUnlock()

	if m.filePath == "" {
		return "", "", false
	}

	return m.filePath, m.originalName, true
}

// 10GB
const maxUploadSize = 10 << 30

func uploadHandler(state *MovieState, room *Room) http.HandlerFunc {
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

		// validating mp4 strcuture and seeakabilty
		if err := validateMP4(tempFile); err != nil {
			_ = os.Remove(tempFile.Name())
			http.Error(w, "Upload rejected: "+err.Error(), http.StatusBadRequest)
			return
		}

		state.Set(tempFile.Name(), header.Filename)

		room.PlayBack.Update(0, false)

		room.Broadcast(Message{
			Type:      "MOVIE_CHANGED",
			Position:  0,
			IsPlaying: false,
			Count:     room.ClientCount(),
		})

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{
			"message":   "Uploaded successfully",
			"filename":  header.Filename,
			"roomID":    room.ID,
			"hostToken": room.HostToken,
		})
	}
}

func infoHandler(state *MovieState, room *Room) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		_, name, hasMovie := state.Get()

		w.Header().Set("Content-Type", "application/json")

		json.NewEncoder(w).Encode(map[string]any{
			"roomId":   room.ID,
			"hasMovie": hasMovie,
			"filename": name,
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

// validateMP4 verifies that the file is an MP4 and has faststart (moov before mdat)
func validateMP4(file *os.File) error {
	var header [8]byte

	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return err
	}

	// 1. Check if first box is 'ftyp'
	if _, err := io.ReadFull(file, header[:]); err != nil {
		return fmt.Errorf("file too small")
	}
	if string(header[4:8]) != "ftyp" {
		return fmt.Errorf("invalid format: file must be an MP4 (missing ftyp)")
	}

	// 2. Scan top-level boxes to check moov vs mdat order
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return err
	}

	var seenMdat bool
	for {
		if _, err := io.ReadFull(file, header[:]); err != nil {
			break // Reached EOF
		}

		size := binary.BigEndian.Uint32(header[0:4])
		boxType := string(header[4:8])

		if boxType == "moov" {
			if seenMdat {
				return fmt.Errorf("MP4 lacks FastStart: 'mdat' appears before 'moov' (seeking will fail)")
			}
			return nil // Valid MP4 with FastStart!
		}
		if boxType == "mdat" {
			seenMdat = true
		}

		if size < 8 {
			break
		}
		// Skip to the next box
		if _, err := file.Seek(int64(size-8), io.SeekCurrent); err != nil {
			break
		}
	}

	return fmt.Errorf("invalid MP4: missing 'moov' metadata atom")
}

func main() {
	// http.HandleFunc("/stream",streamHandler("song.mp3"))
	moviestate := &MovieState{}
	room := NewRoom()

	log.Printf("Room created..ID : %s (Host Token: %s)", room.ID, room.HostToken)

	mux := http.NewServeMux()
	mux.HandleFunc("/stream", streamHandler(moviestate))
	mux.HandleFunc("/api/upload", uploadHandler(moviestate, room))
	mux.HandleFunc("/api/info", infoHandler(moviestate, room))
	mux.Handle("/", http.FileServer(http.Dir("./static")))
	mux.HandleFunc("/ws", wsHandler(room))

	server := &http.Server{
		Addr:    ":6969",
		Handler: mux,
	}

	go func() {
		// log.Println("Server running on http://localhost:6969")
		PrintBanner("6969", room.ID)
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
