package upload

import (
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"

	"github.com/jackc/pgx/v5/pgxpool"

	"streaming-service/internal/transcoder"
)

// Handler handles video uploads.
type Handler struct {
	pool         *pgxpool.Pool
	videosDir    string
	transcodeDir string
	streamBase   string
}

func NewHandler(pool *pgxpool.Pool, videosDir, transcodeDir, streamBase string) *Handler {
	return &Handler{
		pool:         pool,
		videosDir:    videosDir,
		transcodeDir: transcodeDir,
		streamBase:   streamBase,
	}
}

// ServeHTTP handles POST /upload.
// Accepts multipart/form-data with fields:
//   - file: the video file (.mp4)
//   - series_id: int64
//   - title: string (optional)
//   - episode_num: int32 (optional)
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if err := r.ParseMultipartForm(500 << 20); err != nil {
		http.Error(w, "failed to parse form", http.StatusBadRequest)
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		http.Error(w, "file field required", http.StatusBadRequest)
		return
	}
	defer file.Close()

	seriesIDStr := r.FormValue("series_id")
	if seriesIDStr == "" {
		http.Error(w, "series_id required", http.StatusBadRequest)
		return
	}
	seriesID, err := strconv.ParseInt(seriesIDStr, 10, 64)
	if err != nil {
		http.Error(w, "invalid series_id", http.StatusBadRequest)
		return
	}

	title := r.FormValue("title")

	var episodeNum *int32
	if s := r.FormValue("episode_num"); s != "" {
		v, err := strconv.ParseInt(s, 10, 32)
		if err != nil {
			http.Error(w, "invalid episode_num", http.StatusBadRequest)
			return
		}
		v32 := int32(v)
		episodeNum = &v32
	}

	videoID := header.Filename
	if ext := filepath.Ext(videoID); ext != "" {
		videoID = videoID[:len(videoID)-len(ext)]
	}

	videoPath := filepath.Join(h.videosDir, videoID+".mp4")
	dst, err := os.Create(videoPath)
	if err != nil {
		log.Printf("create video file: %v", err)
		http.Error(w, "failed to save video", http.StatusInternalServerError)
		return
	}
	defer dst.Close()

	if _, err := io.Copy(dst, file); err != nil {
		log.Printf("copy video data: %v", err)
		http.Error(w, "failed to save video", http.StatusInternalServerError)
		return
	}

	outputDir := filepath.Join(h.transcodeDir, videoID)
	if _, err := transcoder.TranscodeToHLS(videoPath, outputDir); err != nil {
		log.Printf("transcode error: %v", err)
		http.Error(w, "transcoding failed", http.StatusInternalServerError)
		return
	}

	// Original video больше не нужен — HLS уже готов
	if err := os.Remove(videoPath); err != nil {
		log.Printf("warning: failed to remove original video %s: %v", videoPath, err)
	}

	streamURL := fmt.Sprintf("%s/%s/index.m3u8", h.streamBase, videoID)
	log.Printf("Stream URL: %s", streamURL)

	// Avoid duplicates — if an episode with the same stream URL already exists for this series, return it
	var episodeID int64
	err = h.pool.QueryRow(r.Context(),
		`SELECT id FROM episodes WHERE series_id = $1 AND tiktok_url = $2`,
		seriesID, streamURL,
	).Scan(&episodeID)
	if err != nil {
		episodeID, err = h.createEpisode(r.Context(), seriesID, title, streamURL, episodeNum)
		if err != nil {
			log.Printf("create episode in DB: %v", err)
			http.Error(w, "failed to create episode record", http.StatusInternalServerError)
			return
		}
		log.Printf("Episode %d created for series %d: %s", episodeID, seriesID, streamURL)
	} else {
		log.Printf("Episode %d already exists for series %d, returning existing", episodeID, seriesID)
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	fmt.Fprintf(w, `{"episode_id":%d,"stream_url":"%s"}`, episodeID, streamURL)
}

func (h *Handler) createEpisode(ctx context.Context, seriesID int64, title, streamURL string, episodeNum *int32) (int64, error) {
	query := `INSERT INTO episodes (series_id, title, tiktok_url, episode_num) VALUES ($1, $2, $3, $4) RETURNING id`

	var titlePtr *string
	if title != "" {
		titlePtr = &title
	}

	var rowID int64
	err := h.pool.QueryRow(ctx, query, &seriesID, titlePtr, streamURL, episodeNum).Scan(&rowID)
	return rowID, err
}
