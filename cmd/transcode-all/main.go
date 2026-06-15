package main

import (
	"context"
	"encoding/csv"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/pprAImm/database"

	"streaming-service/internal/transcoder"
)

func main() {
	log.SetFlags(log.Ltime | log.Lmsgprefix)
	log.SetPrefix("")

	if len(os.Args) < 2 {
		log.Fatalf("Использование: %s <episodes.csv>\n\n"+
			"Формат CSV:\n"+
			"  filename,series_title,category_slug,episode_title,episode_num\n"+
			"  video1.mp4,Мой сериал,comedy,Первая серия,1\n"+
			"\n"+
			"Если сериал с таким названием не найден — он будет создан.\n"+
			"Для создания сериала нужен category_slug (из таблицы categories).\n"+
			"\n"+
			"Переменные окружения:\n"+
			"  VIDEOS_DIR, TRANSCODE_DIR, STREAM_BASE_URL, DATABASE_URL\n",
			os.Args[0])
	}

	videosDir := getEnv("VIDEOS_DIR", "videos")
	transcodeDir := getEnv("TRANSCODE_DIR", "transcoded")
	streamBase := getEnv("STREAM_BASE_URL", "http://localhost:8082/stream")

	pool, err := database.Init()
	if err != nil {
		log.Fatalf("db init: %v", err)
	}
	defer pool.Close()

	csvPath := os.Args[1]
	f, err := os.Open(csvPath)
	if err != nil {
		log.Fatalf("open CSV: %v", err)
	}
	defer f.Close()

	reader := csv.NewReader(f)
	reader.FieldsPerRecord = -1 // allow variable columns
	reader.TrimLeadingSpace = true

	header, err := reader.Read()
	if err != nil {
		log.Fatalf("read CSV header: %v", err)
	}

	colIdx := buildColIndex(header)

	var processed, skipped, failed int

	for {
		record, err := reader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			log.Fatalf("read CSV row: %v", err)
		}

		filename := getCol(record, colIdx, "filename")
		seriesTitle := getCol(record, colIdx, "series_title")
		categorySlug := getCol(record, colIdx, "category_slug")
		episodeTitle := getCol(record, colIdx, "episode_title")
		episodeNumStr := getCol(record, colIdx, "episode_num")

		if filename == "" || seriesTitle == "" {
			log.Printf("[SKIP] пропущена строка: filename или series_title пусты")
			skipped++
			continue
		}

		videoID := strings.TrimSuffix(filename, filepath.Ext(filename))
		inputPath := filepath.Join(videosDir, filename)
		outputDir := filepath.Join(transcodeDir, videoID)

		log.Printf("--- %s ---", filename)

	alreadyTranscoded := transcoder.IsTranscoded(outputDir)

	if !alreadyTranscoded {
		if _, err := os.Stat(inputPath); os.IsNotExist(err) {
			log.Printf("[SKIP] файл не найден и HLS отсутствует: %s", inputPath)
			skipped++
			continue
		}

		log.Printf("[TRANSCODE] %s", filename)
		if _, err := transcoder.TranscodeToHLS(inputPath, outputDir); err != nil {
			log.Printf("[FAIL] %s: %v", filename, err)
			failed++
			continue
		}
		if err := os.Remove(inputPath); err != nil {
			log.Printf("[WARN] не удалось удалить оригинал %s: %v", filename, err)
		}
	} else {
		log.Printf("[SKIP HLS] уже обработан")
	}

		seriesID, err := findOrCreateSeries(pool, seriesTitle, categorySlug)
		if err != nil {
			log.Printf("[FAIL] series: %v", err)
			failed++
			continue
		}

		streamURL := fmt.Sprintf("%s/%s/index.m3u8", streamBase, videoID)

		var episodeNum *int32
		if episodeNumStr != "" {
			v, err := strconv.ParseInt(episodeNumStr, 10, 32)
			if err == nil {
				v32 := int32(v)
				episodeNum = &v32
			}
		}

		var titlePtr *string
		if episodeTitle != "" {
			titlePtr = &episodeTitle
		}

		var episodeID int64
		err = pool.QueryRow(context.Background(), `INSERT INTO episodes (series_id, title, tiktok_url, episode_num)
			VALUES ($1, $2, $3, $4) RETURNING id`, seriesID, titlePtr, streamURL, episodeNum).Scan(&episodeID)
		if err != nil {
			log.Printf("[FAIL] create episode: %v", err)
			failed++
			continue
		}

		log.Printf("[DONE] эпизод %d → %s", episodeID, streamURL)
		processed++
	}

	fmt.Printf("\nГотово: %d обработано, %d пропущено, %d ошибок\n", processed, skipped, failed)
}

func findOrCreateSeries(pool *pgxpool.Pool, title, categorySlug string) (int64, error) {
	var id int64
	err := pool.QueryRow(context.Background(), `SELECT id FROM series WHERE title = $1`, title).Scan(&id)
	if err == nil {
		return id, nil
	}

	// Not found — create
	var categoryID int64
	err = pool.QueryRow(context.Background(), `SELECT id FROM categories WHERE slug = $1`, categorySlug).Scan(&categoryID)
	if err != nil {
		return 0, fmt.Errorf("категория с slug %q не найдена. Доступные: Фрукты(fruits), Азиатское(asian), Романтика(romance), Комедия(comedy), Ужасы(horror), Драма(drama)", categorySlug)
	}

	err = pool.QueryRow(context.Background(),
		`INSERT INTO series (title, description, category_id, cover_url) VALUES ($1, '', $2, '') RETURNING id`,
		title, categoryID).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("create series: %w", err)
	}

	log.Printf("[NEW SERIES] создан сериал %q (id=%d)", title, id)
	return id, nil
}

func buildColIndex(header []string) map[string]int {
	idx := make(map[string]int)
	for i, h := range header {
		idx[strings.TrimSpace(strings.ToLower(h))] = i
	}
	return idx
}

func getCol(record []string, idx map[string]int, col string) string {
	if i, ok := idx[col]; ok && i < len(record) {
		return strings.TrimSpace(record[i])
	}
	return ""
}

func getEnv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
