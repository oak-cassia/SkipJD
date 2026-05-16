// Package extractor classifies job posting bodies into experience /
// competency / trait via the local gemini-cli and writes one row per posting
// into job_posting_extractions.
package extractor

import (
	"context"
	"fmt"
	"log"
	"os"
	"sync/atomic"
	"time"

	"skipjd/internal/batch"
	"skipjd/internal/geminiexec"
	"skipjd/internal/repository"
)

// Model is the gemini model name passed via `gemini -m` and stored in the
// JobPostingExtraction.Model column for traceability.
const Model = "gemini-2.5-pro"

// Options bundles the CLI flags the cmd entry point passes through.
type Options struct {
	Limit         int
	Offset        int
	DebugDir      string
	Workers       int
	GeminiTimeout time.Duration
}

// Run processes up to opts.Limit pending bodies.
// Failures are logged and skipped — the next batch run will retry naturally
// because no failure state is persisted.
func Run(ctx context.Context, repo *repository.CrawlerRepository, opts Options) error {
	if err := geminiexec.EnsureAvailable(); err != nil {
		return err
	}

	pending, err := repo.FetchPendingExtractions(ctx, opts.Limit, opts.Offset)
	if err != nil {
		return fmt.Errorf("fetch pending: %w", err)
	}
	log.Printf("pending postings=%d", len(pending))

	if opts.DebugDir != "" {
		if err := os.MkdirAll(opts.DebugDir, 0o755); err != nil {
			return fmt.Errorf("create debug dir: %w", err)
		}
	}

	var success, failed atomic.Int64

	batch.Run(ctx, pending, opts.Workers, func(egCtx context.Context, b repository.PendingBody) {
		if processBody(egCtx, repo, b, opts) {
			success.Add(1)
		} else {
			failed.Add(1)
		}
	})

	log.Printf("done success=%d failed=%d total=%d", success.Load(), failed.Load(), len(pending))
	return nil
}

func processBody(ctx context.Context, repo *repository.CrawlerRepository, body repository.PendingBody, opts Options) bool {
	raw, err := geminiexec.Call(ctx, geminiexec.CallOptions{
		Prompt:   promptTemplate,
		Input:    body.Text,
		Model:    Model,
		Label:    fmt.Sprintf("posting_id=%d", body.JobPostingID),
		DebugDir: opts.DebugDir,
		Timeout:  opts.GeminiTimeout,
	})
	if err != nil {
		log.Printf("gemini failed posting_id=%d err=%v", body.JobPostingID, err)
		return false
	}

	result, err := geminiexec.ParseResponse(raw)
	if err != nil {
		log.Printf("parse failed posting_id=%d preview=%q", body.JobPostingID, geminiexec.Preview(raw, 120))
		return false
	}

	expJSON, err := geminiexec.EncodeArray(result.Experience)
	if err != nil {
		log.Printf("encode experience posting_id=%d err=%v", body.JobPostingID, err)
		return false
	}
	compJSON, err := geminiexec.EncodeArray(result.Competency)
	if err != nil {
		log.Printf("encode competency posting_id=%d err=%v", body.JobPostingID, err)
		return false
	}
	traitJSON, err := geminiexec.EncodeArray(result.Trait)
	if err != nil {
		log.Printf("encode trait posting_id=%d err=%v", body.JobPostingID, err)
		return false
	}

	if err := repo.UpsertJobPostingExtraction(
		ctx,
		body.JobPostingID,
		expJSON, compJSON, traitJSON,
		Model,
		body.BodyUpdatedAt,
	); err != nil {
		log.Printf("upsert failed posting_id=%d err=%v", body.JobPostingID, err)
		return false
	}
	return true
}

