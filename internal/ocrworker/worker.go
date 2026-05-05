// Package ocrworker is a batch worker that runs OCR over images attached to
// job postings via the local gemini-cli and persists the resulting text into
// job_posting_bodies.
package ocrworker

import (
	"context"
	"fmt"
	"log"
	"os"
	"strings"
	"sync/atomic"
	"time"
	"unicode/utf8"

	"golang.org/x/sync/errgroup"

	"skipjd/internal/geminiexec"
	"skipjd/internal/model"
	"skipjd/internal/repository"
)

// Options bundles the CLI flags the cmd entry point passes through.
type Options struct {
	Limit         int
	Offset        int
	MinOCRChars   int
	DebugDir      string
	Workers       int
	GeminiTimeout time.Duration
}

// Run processes up to opts.Limit pending postings.
// Failures are logged and skipped — the next batch run will retry naturally
// because no failure state is persisted.
func Run(ctx context.Context, repo *repository.CrawlerRepository, opts Options) error {
	if err := geminiexec.EnsureAvailable(); err != nil {
		return err
	}

	ids, err := repo.FetchPendingOCRPostingIDs(ctx, opts.Limit, opts.Offset)
	if err != nil {
		return fmt.Errorf("fetch pending: %w", err)
	}
	log.Printf("pending postings=%d", len(ids))

	if opts.DebugDir != "" {
		if err := os.MkdirAll(opts.DebugDir, 0o755); err != nil {
			return fmt.Errorf("create debug dir: %w", err)
		}
	}

	var success, empty atomic.Int64

	workers := opts.Workers
	if workers <= 0 {
		workers = 3
	}

	eg, egCtx := errgroup.WithContext(ctx)
	eg.SetLimit(workers)

	for _, id := range ids {
		pID := id
		eg.Go(func() error {
			images, err := repo.FetchImagesForPosting(egCtx, pID)
			if err != nil {
				log.Printf("fetch images failed posting_id=%d err=%v", pID, err)
				return nil
			}

			ocrText := processPosting(egCtx, pID, images, opts)
			if ocrText == "" {
				empty.Add(1)
				log.Printf("empty result posting_id=%d", pID)
				return nil
			}

			if err := repo.UpsertJobPostingBodyOCR(egCtx, pID, ocrText); err != nil {
				log.Printf("upsert failed posting_id=%d err=%v", pID, err)
				return nil
			}
			success.Add(1)
			return nil
		})
	}

	_ = eg.Wait()

	log.Printf("done success=%d empty=%d total=%d", success.Load(), empty.Load(), len(ids))
	return nil
}

func processPosting(ctx context.Context, postingID uint, images []model.JobPostingImage, opts Options) string {
	parts := make([]string, 0, len(images))
	for i, img := range images {
		text, ok := ocrImage(ctx, img.ImageURL, postingID, i, opts)
		if !ok {
			continue
		}
		parts = append(parts, text)
	}
	return strings.TrimSpace(strings.Join(parts, "\n\n"))
}

func ocrImage(ctx context.Context, imageURL string, postingID uint, idx int, opts Options) (string, bool) {
	payload, err := downloadImage(ctx, imageURL)
	if err != nil {
		log.Printf("download skipped posting_id=%d url=%s err=%v", postingID, imageURL, err)
		return "", false
	}

	text, err := ocrPayload(ctx, payload, postingID, idx, opts.DebugDir, opts.GeminiTimeout)
	if err != nil {
		log.Printf("ocr failed posting_id=%d url=%s err=%v", postingID, imageURL, err)
		return "", false
	}

	text = strings.TrimSpace(text)
	chars := utf8.RuneCountInString(text)
	if chars < opts.MinOCRChars {
		log.Printf("skip ocr_text_too_short posting_id=%d url=%s chars=%d", postingID, imageURL, chars)
		return "", false
	}
	return text, true
}
