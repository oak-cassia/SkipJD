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
	"flag"
	"log"
	"time"

	"skipjd/internal/cmdutil"
	"skipjd/internal/extractor"
	"skipjd/internal/repository"
)

func main() {
	limit := flag.Int("limit", 50, "max postings per run")
	offset := flag.Int("offset", 0, "skip the first N postings (useful for debugging)")
	debugDir := flag.String("debug-dir", "", "directory to retain body files instead of using temp files")
	workers := flag.Int("workers", 3, "Number of concurrent gemini calls")
	geminiTimeout := flag.Duration("gemini-timeout", 0, "Timeout per gemini call (0 = default)")
	deadline := flag.Duration("deadline", 0, "Max duration for the extractor batch (0 = no deadline)")
	flag.Parse()

	opts := extractor.Options{
		Limit:         *limit,
		Offset:        *offset,
		DebugDir:      *debugDir,
		Workers:       *workers,
		GeminiTimeout: *geminiTimeout,
	}
	if err := run(*deadline, opts); err != nil {
		log.Fatalf("extractor failed: %v", err)
	}
}

// run is split from main so log.Fatalf cannot skip the deferred cancel.
func run(deadline time.Duration, opts extractor.Options) error {
	ctx, cancel := cmdutil.SetupContext(deadline)
	defer cancel()

	repo := repository.NewCrawlerRepository(cmdutil.MustConnectDB())
	return extractor.Run(ctx, repo, opts)
}
