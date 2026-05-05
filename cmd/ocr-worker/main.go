// ocr-worker is a batch worker that runs OCR over images attached to job
// postings via the local gemini-cli and persists the resulting text into
// job_posting_bodies (source=ocr).
//
// Reads DB config from project root .env (DB_USER, DB_PASS, DB_HOST,
// DB_PORT, DB_NAME, REQUIRE_DB_TLS) and requires `gemini` on PATH.
//
// Usage:
//
//	go run ./cmd/ocr-worker [--limit N] [--offset N] [--min-ocr-chars N] [--debug-dir DIR]
package main

import (
	"context"
	"flag"
	"log"

	"github.com/joho/godotenv"

	"skipjd/internal/config"
	"skipjd/internal/database"
	"skipjd/internal/ocrworker"
	"skipjd/internal/repository"
)

func main() {
	limit := flag.Int("limit", 50, "max postings per run")
	offset := flag.Int("offset", 0, "skip the first N postings (useful for debugging)")
	minChars := flag.Int("min-ocr-chars", 20, "skip OCR results shorter than this length (assumes failure/garbage)")
	debugDir := flag.String("debug-dir", "", "directory to retain image and OCR text files instead of using temp files")
	workers := flag.Int("workers", 3, "Number of concurrent OCR worker routines")
	geminiTimeout := flag.Duration("gemini-timeout", 0, "Timeout per gemini call (0 = default)")
	deadline := flag.Duration("deadline", 0, "Max duration for the ocr-worker batch (0 = no deadline)")
	flag.Parse()

	_ = godotenv.Load()

	ctx := context.Background()
	if *deadline > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, *deadline)
		defer cancel()
	}

	db, err := database.NewGormDB(config.LoadDatabaseConfig())
	if err != nil {
		log.Fatalf("failed to connect db: %v", err)
	}

	repo := repository.NewCrawlerRepository(db)
	if err := ocrworker.Run(ctx, repo, ocrworker.Options{
		Limit:         *limit,
		Offset:        *offset,
		MinOCRChars:   *minChars,
		DebugDir:      *debugDir,
		Workers:       *workers,
		GeminiTimeout: *geminiTimeout,
	}); err != nil {
		log.Fatalf("ocr worker failed: %v", err)
	}
}
