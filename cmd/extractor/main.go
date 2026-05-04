// extractor is a batch worker that classifies job posting bodies into
// experience / competency / trait via the local gemini-cli and writes one
// row per posting into job_posting_extractions.
//
// Reads DB config from project root .env (DB_USER, DB_PASS, DB_HOST,
// DB_PORT, DB_NAME, REQUIRE_DB_TLS) and requires `gemini` on PATH.
//
// Usage:
//
//	go run ./cmd/extractor [--limit N] [--offset N] [--debug-dir DIR]
package main

import (
	"context"
	"flag"
	"log"

	"github.com/joho/godotenv"

	"skipjd/internal/config"
	"skipjd/internal/database"
	"skipjd/internal/extractor"
	"skipjd/internal/repository"
)

func main() {
	limit := flag.Int("limit", 50, "max postings per run")
	offset := flag.Int("offset", 0, "skip the first N postings (useful for debugging)")
	debugDir := flag.String("debug-dir", "", "directory to retain body files instead of using temp files")
	flag.Parse()

	_ = godotenv.Load()

	db, err := database.NewGormDB(config.LoadDatabaseConfig())
	if err != nil {
		log.Fatalf("failed to connect db: %v", err)
	}

	repo := repository.NewCrawlerRepository(db)
	if err := extractor.Run(context.Background(), repo, extractor.Options{
		Limit:    *limit,
		Offset:   *offset,
		DebugDir: *debugDir,
	}); err != nil {
		log.Fatalf("extractor failed: %v", err)
	}
}
