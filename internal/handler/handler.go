package handler

import (
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"streaming-service/internal/storage"
	"streaming-service/internal/transcoder"
)

// Handler serves HLS video streams.
type Handler struct {
	storage storage.Storage
}

func NewHandler(s storage.Storage) *Handler {
	return &Handler{storage: s}
}

// ServeHLS handles /stream/{videoID}/* requests.
//   - /stream/{videoID}/index.m3u8  → HLS playlist
//   - /stream/{videoID}/segment_001.ts → MPEG-TS segment
func (h *Handler) ServeHLS(w http.ResponseWriter, r *http.Request) {
	// Path: /stream/{videoID}/filename
	parts := strings.SplitN(strings.TrimPrefix(r.URL.Path, "/stream/"), "/", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		http.Error(w, "invalid path", http.StatusBadRequest)
		return
	}

	videoID := parts[0]
	filename := parts[1]

	outputDir := h.storage.TranscodeDir(videoID)

	// Transcode on first access
	if !transcoder.IsTranscoded(outputDir) {
		log.Printf("Transcoding video %s...", videoID)

		src, err := h.storage.Open(videoID)
		if err != nil {
			http.Error(w, "video not found", http.StatusNotFound)
			return
		}
		src.Close() // we only need the path check

		// Use a local file path — for production swap this with S3 download
		inputPath := filepath.Join("videos", videoID+".mp4")
		if _, err := os.Stat(inputPath); err != nil {
			http.Error(w, "source file not found", http.StatusNotFound)
			return
		}

		if _, err := transcoder.TranscodeToHLS(inputPath, outputDir); err != nil {
			log.Printf("Transcode error: %v", err)
			http.Error(w, "transcoding failed", http.StatusInternalServerError)
			return
		}
		log.Printf("Transcoded %s -> %s", videoID, outputDir)
	}

	filePath := filepath.Join(outputDir, filename)
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		http.Error(w, "file not found", http.StatusNotFound)
		return
	}

	// Set correct MIME types
	switch {
	case strings.HasSuffix(filename, ".m3u8"):
		w.Header().Set("Content-Type", "application/vnd.apple.mpegurl")
	case strings.HasSuffix(filename, ".ts"):
		w.Header().Set("Content-Type", "video/mp2t")
	}

	w.Header().Set("Cache-Control", "public, max-age=86400")
	http.ServeFile(w, r, filePath)
}

// ServeVideo handles /videos/{videoID} for direct video access.
func (h *Handler) ServeVideo(w http.ResponseWriter, r *http.Request) {
	videoID := strings.TrimPrefix(r.URL.Path, "/videos/")
	if videoID == "" {
		http.Error(w, "invalid path", http.StatusBadRequest)
		return
	}

	outputDir := h.storage.TranscodeDir(videoID)

	if !transcoder.IsTranscoded(outputDir) {
		inputPath := filepath.Join("videos", videoID+".mp4")
		if _, err := os.Stat(inputPath); err != nil {
			http.Error(w, "video not found", http.StatusNotFound)
			return
		}

		if _, err := transcoder.TranscodeToHLS(inputPath, outputDir); err != nil {
			log.Printf("Transcode error: %v", err)
			http.Error(w, "transcoding failed", http.StatusInternalServerError)
			return
		}
	}

	// Redirect to the HLS playlist
	http.Redirect(w, r, "/stream/"+videoID+"/index.m3u8", http.StatusFound)
}
