package storage

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// Storage provides access to video source files.
type Storage interface {
	// Open opens a video file for reading.
	Open(videoID string) (io.ReadCloser, error)

	// TranscodeDir returns the directory for HLS output.
	TranscodeDir(videoID string) string
}

// LocalStorage stores videos on the local filesystem.
type LocalStorage struct {
	VideosDir      string
	TranscodeRoot  string
}

func NewLocalStorage(videosDir, transcodeRoot string) *LocalStorage {
	return &LocalStorage{
		VideosDir:     videosDir,
		TranscodeRoot: transcodeRoot,
	}
}

func (s *LocalStorage) Open(videoID string) (io.ReadCloser, error) {
	path := filepath.Join(s.VideosDir, videoID+".mp4")
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open video %s: %w", videoID, err)
	}
	return f, nil
}

func (s *LocalStorage) TranscodeDir(videoID string) string {
	return filepath.Join(s.TranscodeRoot, videoID)
}
