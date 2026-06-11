// Package userextractor classifies a user's resume / self-introduction text
// into experience / competency / trait via the local gemini-cli and writes
// one row per user into user_extractions. The output shape mirrors
// internal/extractor so user and JD can be matched on identical axes.
package userextractor

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"skipjd/internal/geminiexec"
	"skipjd/internal/repository"
)

// Options bundles the CLI flags the cmd entry point passes through.
type Options struct {
	UserID        uint
	Files         []string
	DebugDir      string
	GeminiTimeout time.Duration
	Force         bool
}

// Run reads the resume files at opts.Files (concatenating them when more
// than one is supplied), extracts experience / competency / trait via
// gemini-cli, and upserts a user_extractions row.
//
// Idempotency: when the SHA256 of the combined input text matches the
// stored SourceHash for this user, the gemini call is skipped unless
// --force is set.
func Run(ctx context.Context, repo *repository.UserExtractionRepository, opts Options) error {
	if opts.UserID == 0 {
		return errors.New("user-id must be > 0")
	}
	if len(opts.Files) == 0 {
		return errors.New("--file is required (one or more)")
	}
	if err := geminiexec.EnsureAvailable(); err != nil {
		return err
	}

	resumeText, err := ReadResumes(opts.Files)
	if err != nil {
		return err
	}
	if strings.TrimSpace(resumeText) == "" {
		return fmt.Errorf("resume text is empty (files=%v); image-only PDFs are not supported", opts.Files)
	}

	sum := sha256.Sum256([]byte(resumeText))
	hash := hex.EncodeToString(sum[:])

	if !opts.Force {
		var existing string
		existing, err = repo.GetSourceHash(ctx, opts.UserID)
		if err != nil {
			return fmt.Errorf("check existing hash: %w", err)
		}
		if existing == hash {
			log.Printf("skip: source unchanged user_id=%d hash=%s", opts.UserID, hash[:12])
			return nil
		}
	}

	if opts.DebugDir != "" {
		if err = os.MkdirAll(opts.DebugDir, 0o755); err != nil {
			return fmt.Errorf("create debug dir: %w", err)
		}
	}

	raw, err := geminiexec.Call(ctx, geminiexec.CallOptions{
		Prompt:   promptTemplate,
		Input:    resumeText,
		Model:    Model,
		Label:    fmt.Sprintf("user_id=%d", opts.UserID),
		DebugDir: opts.DebugDir,
		Timeout:  opts.GeminiTimeout,
	})
	if err != nil {
		return fmt.Errorf("gemini: %w", err)
	}

	result, err := geminiexec.ParseResponse(raw)
	if err != nil {
		return fmt.Errorf("parse response: %w (preview=%q)", err, geminiexec.Preview(raw, 120))
	}

	expJSON, err := geminiexec.EncodeArray(result.Experience)
	if err != nil {
		return fmt.Errorf("encode experience: %w", err)
	}
	compJSON, err := geminiexec.EncodeArray(result.Competency)
	if err != nil {
		return fmt.Errorf("encode competency: %w", err)
	}
	traitJSON, err := geminiexec.EncodeArray(result.Trait)
	if err != nil {
		return fmt.Errorf("encode trait: %w", err)
	}

	if err := repo.UpsertUserExtraction(
		ctx,
		opts.UserID,
		expJSON, compJSON, traitJSON,
		Model,
		strings.Join(opts.Files, ";"),
		hash,
	); err != nil {
		return fmt.Errorf("upsert: %w", err)
	}

	log.Printf("done user_id=%d experience=%d competency=%d trait=%d hash=%s",
		opts.UserID, len(result.Experience), len(result.Competency), len(result.Trait), hash[:12])
	return nil
}

