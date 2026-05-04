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

	"skipjd/internal/repository"
)

// Model is the gemini model name passed via `gemini -m` and stored in the
// JobPostingExtraction.Model column for traceability.
const Model = "gemini-2.5-pro"

// Options bundles the CLI flags the cmd entry point passes through.
type Options struct {
	Limit    int
	Offset   int
	DebugDir string
}

// Run processes up to opts.Limit pending bodies sequentially. Per-row
// failures are logged and skipped — the next batch run will retry naturally
// because no failure state is persisted.
func Run(ctx context.Context, repo *repository.CrawlerRepository, opts Options) error {
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

	success, failed := 0, 0
	for _, body := range pending {
		if processBody(ctx, repo, body, opts.DebugDir) {
			success++
		} else {
			failed++
		}
	}

	log.Printf("done success=%d failed=%d total=%d", success, failed, len(pending))
	return nil
}

func processBody(ctx context.Context, repo *repository.CrawlerRepository, body repository.PendingBody, debugDir string) bool {
	raw, err := callGemini(ctx, body.Text, body.JobPostingID, debugDir)
	if err != nil {
		log.Printf("gemini failed posting_id=%d err=%v", body.JobPostingID, err)
		return false
	}

	result, err := parseResponse(raw)
	if err != nil {
		log.Printf("parse failed posting_id=%d preview=%q", body.JobPostingID, previewLine(raw, 120))
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
