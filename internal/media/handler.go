package media

import (
	"encoding/json"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"

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

		ext := strings.ToLower(header.Filename)
		isMKV := strings.HasSuffix(ext, ".mkv")

		pattern := "streamla-*.mp4"
		if isMKV {
			pattern = "streamla-*.mkv"
		}

		tempFile, err := os.CreateTemp("", pattern)
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

		// If MP4, check FastStart box structure
		if !isMKV {
			if err := ValidateMP4(tempFile); err != nil {
				_ = os.Remove(tempFile.Name())
				http.Error(w, "Upload rejected: "+err.Error(), http.StatusBadRequest)
				return
			}
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

// InfoHandler returns current room metadata, loaded movie info, audio tracks, and subtitles.
func InfoHandler(state *MovieState, r *room.Room) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		_, name, probe, hasMovie := state.Get()

		var audioTracks []StreamInfo
		var subtitles []StreamInfo
		var duration float64

		if probe != nil {
			audioTracks = probe.Audio
			subtitles = probe.Subtitles
			duration = probe.Duration
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"roomId":      r.ID,
			"hasMovie":    hasMovie,
			"filename":    name,
			"duration":    duration,
			"audioTracks": audioTracks,
			"subtitles":   subtitles,
		})
	}
}

// StreamHandler serves media either via direct byte ranges or on-the-fly stream remuxing.
func StreamHandler(state *MovieState) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		path, _, probe, ok := state.Get()
		if !ok {
			http.Error(w, "No movie currently selected", http.StatusNotFound)
			return
		}

		// Check if an alternate audio track was requested
		audioParam := req.URL.Query().Get("audio")
		var requestedAudioIndex = -1
		if audioParam != "" {
			if idx, err := strconv.Atoi(audioParam); err == nil {
				requestedAudioIndex = idx
			}
		}

		needsRemux := false
		if probe != nil && probe.NeedsRemux {
			needsRemux = true
		}
		if requestedAudioIndex >= 0 {
			needsRemux = true
		}

		if needsRemux {
			// Find if requested audio track is AAC
			isAAC := false
			if probe != nil {
				for _, a := range probe.Audio {
					if (requestedAudioIndex >= 0 && a.Index == requestedAudioIndex) ||
						(requestedAudioIndex < 0 && len(probe.Audio) > 0 && a.Index == probe.Audio[0].Index) {
						if a.Codec == "aac" {
							isAAC = true
						}
						break
					}
				}
			}

			_ = StreamRemux(req.Context(), path, requestedAudioIndex, isAAC, w)
			return
		}

		// Direct stream: standard MP4 with browser-native codecs
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

// SubtitleHandler serves on-the-fly extracted WebVTT subtitle tracks.
func SubtitleHandler(state *MovieState) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		path, _, probe, ok := state.Get()
		if !ok {
			http.Error(w, "No active movie", http.StatusNotFound)
			return
		}

		trackParam := req.URL.Query().Get("track")
		if trackParam == "" {
			http.Error(w, "Missing track parameter", http.StatusBadRequest)
			return
		}

		if trackParam == "external" {
			if probe == nil || probe.ExternalSubtitle == "" {
				http.Error(w, "External subtitle not found", http.StatusNotFound)
				return
			}
			_ = ConvertExternalSRT(req.Context(), probe.ExternalSubtitle, w)
			return
		}

		streamIdx, err := strconv.Atoi(trackParam)
		if err != nil {
			http.Error(w, "Invalid track index", http.StatusBadRequest)
			return
		}

		_ = ExtractSubtitleVTT(req.Context(), path, streamIdx, w)
	}
}
