package main

import (
	"log"
	"net/http"
	"os"
	"path/filepath"

	"github.com/pprAImm/database"

	"streaming-service/internal/handler"
	"streaming-service/internal/storage"
	"streaming-service/internal/upload"
)

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8082"
	}

	videosDir := os.Getenv("VIDEOS_DIR")
	if videosDir == "" {
		videosDir = filepath.Join("videos")
	}

	transcodeDir := os.Getenv("TRANSCODE_DIR")
	if transcodeDir == "" {
		transcodeDir = filepath.Join("transcoded")
	}

	if err := os.MkdirAll(videosDir, 0755); err != nil {
		log.Fatalf("create videos dir: %v", err)
	}
	if err := os.MkdirAll(transcodeDir, 0755); err != nil {
		log.Fatalf("create transcode dir: %v", err)
	}

	store := storage.NewLocalStorage(videosDir, transcodeDir)
	h := handler.NewHandler(store)

	// Database connection
	pool, err := database.Init()
	if err != nil {
		log.Fatalf("database init: %v", err)
	}
	defer pool.Close()

	streamBase := os.Getenv("STREAM_BASE_URL")
	if streamBase == "" {
		streamBase = "http://localhost:8082/stream"
	}

	uploadHandler := upload.NewHandler(pool, videosDir, transcodeDir, streamBase)

	mux := http.NewServeMux()
	mux.HandleFunc("/stream/", h.ServeHLS)
	mux.HandleFunc("/videos/", h.ServeVideo)
	mux.Handle("/upload", uploadHandler)

	// Health check
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"status":"ok"}`))
	})

	log.Printf("Streaming service starting on :%s", port)
	log.Printf("  Videos dir: %s", videosDir)
	log.Printf("  Transcode dir: %s", transcodeDir)
	log.Printf("  Stream base: %s", streamBase)

	if err := http.ListenAndServe(":"+port, mux); err != nil {
		log.Fatalf("server error: %v", err)
	}
}
