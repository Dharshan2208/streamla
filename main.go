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

		f, err := os.Open(absPath)
		if err != nil {
			log.Fatalf("Cannot open movie file %q: %v", absPath, err)
		}

		if err := media.ValidateMP4(f); err != nil {
			_ = f.Close()
			log.Fatalf("Movie validation failed for %q: %v", absPath, err)
		}
		_ = f.Close()

		movieState.Set(absPath, filepath.Base(absPath), false)
		log.Printf("Loaded movie from disk: %s", filepath.Base(absPath))
	}

	log.Printf("Room created: ID %s (Host Token: %s)", rm.ID, rm.HostToken)

	// Sub-tree for embedded static web assets
	staticSub, err := fs.Sub(staticFS, "static")
	if err != nil {
		log.Fatalf("Failed to initialize embedded static files: %v", err)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/stream", media.StreamHandler(movieState))
	mux.HandleFunc("/api/upload", media.UploadHandler(movieState, rm))
	mux.HandleFunc("/api/info", media.InfoHandler(movieState, rm))
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
