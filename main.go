package main

import (
	"context"
	"embed"
	"flag"
	"io/fs"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/Dharshan2208/streamla/internal/discovery"
	"github.com/Dharshan2208/streamla/internal/media"
	"github.com/Dharshan2208/streamla/internal/room"
)

//go:embed static/*
var staticFS embed.FS

func main() {
	port := flag.String("port", "6969", "HTTP port to listen on")
	noRemux := flag.Bool("no-remux", false, "Disable automatic FastStart remuxing of MP4 files")
	flag.Parse()

	movieState := media.NewMovieState()
	rm := room.NewRoom()

	// Check if a local video file path was passed via CLI argument
	args := flag.Args()
	if len(args) > 0 {
		filePath := args[0]
		absPath, err := filepath.Abs(filePath)
		if err != nil {
			log.Fatalf("Invalid movie file path: %v", err)
		}

		effectivePath := absPath
		ext := strings.ToLower(filepath.Ext(absPath))
		if ext == ".mp4" || ext == ".m4v" {
			p, _, err := media.EnsureFastStart(absPath, *noRemux)
			if err != nil {
				log.Fatalf("Movie validation failed for %q: %v", absPath, err)
			}
			effectivePath = p
		}

		movieState.Set(effectivePath, filepath.Base(absPath), false)
		log.Printf("Loaded movie from disk: %s", filepath.Base(absPath))
	}

	log.Printf("Room created: ID %s (Host Token: %s)", rm.ID, rm.HostToken)

	// Start room heartbeat loop for continuous drift telemetry
	heartbeatCtx, cancelHeartbeat := context.WithCancel(context.Background())
	defer cancelHeartbeat()
	go rm.StartHeartbeat(heartbeatCtx, time.Second)

	// Sub-tree for embedded static web assets
	staticSub, err := fs.Sub(staticFS, "static")
	if err != nil {
		log.Fatalf("Failed to initialize embedded static files: %v", err)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/stream", media.StreamHandler(movieState))
	mux.HandleFunc("/api/upload", media.UploadHandler(movieState, rm))
	mux.HandleFunc("/api/info", media.InfoHandler(movieState, rm))
	mux.HandleFunc("/api/subtitles", media.SubtitleHandler(movieState))
	mux.HandleFunc("/api/qr", discovery.QRHandler(*port))
	mux.HandleFunc("/ws", room.WSHandler(rm))
	mux.Handle("/", http.FileServer(http.FS(staticSub)))

	server := &http.Server{
		Addr:    ":" + *port,
		Handler: mux,
	}

	go func() {
		discovery.PrintBanner(*port, rm.ID)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Server error: %v", err)
		}
	}()

	stopChan := make(chan os.Signal, 1)
	signal.Notify(stopChan, os.Interrupt, syscall.SIGTERM)
	<-stopChan

	log.Println("Shutting down server...")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_ = server.Shutdown(ctx)
	movieState.Cleanup()
	log.Println("Server exited successfully")
}
