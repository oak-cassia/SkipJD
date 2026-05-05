// Package extractor classifies job posting bodies into experience /
// competency / trait via the local gemini-cli and writes one row per posting
// into job_posting_extractions.
package extractor

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strings"
	"sync/atomic"
	"time"

	"golang.org/x/sync/errgroup"

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

	workers := opts.Workers
	if workers <= 0 {
		workers = 3
	}

	eg, egCtx := errgroup.WithContext(ctx)
	eg.SetLimit(workers)

	for _, body := range pending {
		b := body
		eg.Go(func() error {
			if processBody(egCtx, repo, b, opts) {
				success.Add(1)
			} else {
				failed.Add(1)
			}
			return nil
		})
	}

	_ = eg.Wait()

	log.Printf("done success=%d failed=%d total=%d", success.Load(), failed.Load(), len(pending))
	return nil
}

// TODO: extract gemini call as injectable for unit tests
func processBody(ctx context.Context, repo *repository.CrawlerRepository, body repository.PendingBody, opts Options) bool {
	raw, err := callGemini(ctx, body.Text, body.JobPostingID, opts.DebugDir, opts.GeminiTimeout)
	if err != nil {
		log.Printf("gemini failed posting_id=%d err=%v", body.JobPostingID, err)
		return false
	}

	result, err := parseResponse(raw)
	if err != nil {
		log.Printf("parse failed posting_id=%d preview=%q", body.JobPostingID, previewLine(raw, 120))
		// TODO: route to DLQ table once it exists
		return false
	}

	expJSON, err := encodeArray(result.Experience)
	if err != nil {
		log.Printf("encode experience posting_id=%d err=%v", body.JobPostingID, err)
		return false
	}
	compJSON, err := encodeArray(result.Competency)
	if err != nil {
		log.Printf("encode competency posting_id=%d err=%v", body.JobPostingID, err)
		return false
	}
	traitJSON, err := encodeArray(result.Trait)
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

// encodeArray emits a JSON array with UTF-8 preserved as-is and without
// escaping HTML-unsafe ASCII (<, >, &), so the stored extraction reads the
// same as the source body.
func encodeArray(items []string) (string, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(items); err != nil {
		return "", fmt.Errorf("marshal: %w", err)
	}
	return strings.TrimRight(buf.String(), "\n"), nil
}

func previewLine(s string, n int) string {
	collapsed := strings.ReplaceAll(s, "\n", " ")
	runes := []rune(collapsed)
	if len(runes) > n {
		runes = runes[:n]
	}
	return string(runes)
}
