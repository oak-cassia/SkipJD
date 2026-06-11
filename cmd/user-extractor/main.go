// user-extractor reads one or more resume / self-introduction files (.txt /
// .md / .pdf) and writes one row into user_extractions containing the LLM-
// derived experience / competency / trait arrays — the user-side mirror of
// cmd/extractor.
//
// Reads DB config from project root .env (DB_USER, DB_PASS, DB_HOST,
// DB_PORT, DB_NAME, REQUIRE_DB_TLS) and requires `gemini` on PATH.
//
// Usage:
//
//	go run ./cmd/user-extractor --user-id 1 --file resume.pdf
//	go run ./cmd/user-extractor --user-id 1 --file resume.pdf --file portfolio.pdf
//	go run ./cmd/user-extractor --user-id 1 --file resume.txt --force
package main

import (
	"flag"
	"fmt"
	"log"
	"strings"
	"time"

	"skipjd/internal/cmdutil"
	"skipjd/internal/model"
	"skipjd/internal/repository"
	"skipjd/internal/userextractor"
)

type fileList []string

func (f *fileList) String() string     { return strings.Join(*f, ",") }
func (f *fileList) Set(s string) error { *f = append(*f, s); return nil }

func main() {
	userID := flag.Uint("user-id", 0, "target user id (required, must be > 0)")
	var files fileList
	flag.Var(&files, "file", "path to resume file (.txt, .md, .pdf); repeat for multiple (required)")
	debugDir := flag.String("debug-dir", "", "directory to retain resume text files instead of using temp files")
	geminiTimeout := flag.Duration("gemini-timeout", 0, "per-call gemini timeout (0 = default)")
	deadline := flag.Duration("deadline", 0, "max duration for the whole run (0 = no deadline)")
	force := flag.Bool("force", false, "re-run gemini even if SourceHash matches the stored extraction")
	flag.Parse()

	opts := userextractor.Options{
		UserID:        *userID,
		Files:         files,
		DebugDir:      *debugDir,
		GeminiTimeout: *geminiTimeout,
		Force:         *force,
	}
	if err := run(*deadline, opts); err != nil {
		log.Fatalf("user-extractor failed: %v", err)
	}
}

// run is split from main so log.Fatalf cannot skip the deferred cancel.
func run(deadline time.Duration, opts userextractor.Options) error {
	ctx, cancel := cmdutil.SetupContext(deadline)
	defer cancel()

	db := cmdutil.MustConnectDB()

	if err := db.AutoMigrate(&model.User{}, &model.UserExtraction{}); err != nil {
		return fmt.Errorf("failed to migrate db: %w", err)
	}

	repo := repository.NewUserExtractionRepository(db)
	return userextractor.Run(ctx, repo, opts)
}
