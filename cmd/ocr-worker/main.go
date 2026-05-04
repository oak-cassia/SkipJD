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
	minOCRChars := flag.Int("min-ocr-chars", 20, "discard image OCR result shorter than this many chars")
	debugDir := flag.String("debug-dir", "", "directory to save downloaded images and OCR texts for debugging")
	flag.Parse()

	_ = godotenv.Load()

	db, err := database.NewGormDB(config.LoadDatabaseConfig())
	if err != nil {
		log.Fatalf("failed to connect db: %v", err)
	}

	repo := repository.NewCrawlerRepository(db)
	if err := ocrworker.Run(context.Background(), repo, ocrworker.Options{
		Limit:       *limit,
		Offset:      *offset,
		MinOCRChars: *minOCRChars,
		DebugDir:    *debugDir,
	}); err != nil {
		log.Fatalf("ocr-worker failed: %v", err)
	}
}
