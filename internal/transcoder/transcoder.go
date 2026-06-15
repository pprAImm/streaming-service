package transcoder

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

// playlistName is the filename for the HLS master playlist.
const playlistName = "index.m3u8"

// segmentPattern is the filename pattern for MPEG-TS segments.
const segmentPattern = "segment_%03d.ts"

// HLSOutput contains the paths produced by transcoding.
type HLSOutput struct {
	PlaylistPath string
	OutputDir    string
}

// IsTranscoded checks if HLS output already exists.
func IsTranscoded(outputDir string) bool {
	playlist := filepath.Join(outputDir, playlistName)
	if _, err := os.Stat(playlist); err != nil {
		return false
	}
	return true
}

// TranscodeToHLS converts a video file to H.264 + AAC in MPEG-TS segments
// with an M3U8 playlist using ffmpeg.
func TranscodeToHLS(inputPath, outputDir string) (*HLSOutput, error) {
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return nil, fmt.Errorf("create output dir: %w", err)
	}

	// Skip if already transcoded
	if IsTranscoded(outputDir) {
		return &HLSOutput{
			PlaylistPath: filepath.Join(outputDir, playlistName),
			OutputDir:    outputDir,
		}, nil
	}

	playlistPath := filepath.Join(outputDir, playlistName)
	segPattern := filepath.Join(outputDir, segmentPattern)

	// ffmpeg:
	//   -codec:v libx264   → H.264 (CPU, без GPU)
	//   -preset veryfast   → лёгкий для CPU, но сжимает хорошо
	//   -crf 28            → баланс размера и качества
	//   -codec:a aac -b:a 96k → сжатие аудио
	//   -f hls             → HLS muxer outputs M3U8 playlist
	//   -hls_segment_type mpegts → MPEG-TS container per segment
	//   -hls_time 6        → ~6 sec per segment
	args := []string{
		"-i", inputPath,
		"-codec:v", "libx264",
		"-preset", "veryfast",
		"-crf", "28",
		"-codec:a", "aac",
		"-b:a", "96k",
		"-f", "hls",
		"-hls_time", "6",
		"-hls_list_size", "0",
		"-hls_segment_type", "mpegts",
		"-hls_segment_filename", segPattern,
		playlistPath,
	}

	cmd := exec.Command("ffmpeg", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("ffmpeg transcoding failed: %w", err)
	}

	return &HLSOutput{
		PlaylistPath: playlistPath,
		OutputDir:    outputDir,
	}, nil
}

