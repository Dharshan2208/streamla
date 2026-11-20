package media

import (
	"encoding/json"
	"io"
	"net/http"
	"os"

	"github.com/Dharshan2208/streamla/internal/room"
)

// 10GB maximum upload size
const maxUploadSize = 10 << 30

// UploadHandler processes movie uploads from the web UI.
func UploadHandler(state *MovieState, r *room.Room) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		if req.Method != http.MethodPost {
			http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
			return
		}

		req.Body = http.MaxBytesReader(w, req.Body, maxUploadSize)

		file, header, err := req.FormFile("video")
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
			http.Error(w, "Error saving file: "+err.Error(), http.StatusInternalServerError)
			return
		}

		// Validate MP4 structure and seekability
		if err := ValidateMP4(tempFile); err != nil {
			_ = os.Remove(tempFile.Name())
			http.Error(w, "Upload rejected: "+err.Error(), http.StatusBadRequest)
			return
		}

		state.Set(tempFile.Name(), header.Filename, true)

		r.PlayBack.Update(0, false)

		r.Broadcast(room.Message{
			Type:      "MOVIE_CHANGED",
			Position:  0,
			IsPlaying: false,
			Count:     r.ClientCount(),
		})

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{
			"message":   "Uploaded successfully",
			"filename":  header.Filename,
			"roomID":    r.ID,
			"hostToken": r.HostToken,
		})
	}
}

// InfoHandler returns current room metadata and loaded movie info.
func InfoHandler(state *MovieState, r *room.Room) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		_, name, hasMovie := state.Get()

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"roomId":   r.ID,
			"hasMovie": hasMovie,
			"filename": name,
		})
	}
}

// StreamHandler serves the active movie file with HTTP byte-range support.
func StreamHandler(state *MovieState) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		path, _, ok := state.Get()
		if !ok {
			http.Error(w, "No movie currently selected", http.StatusNotFound)
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
		http.ServeContent(w, req, stat.Name(), stat.ModTime(), file)
	}
}
