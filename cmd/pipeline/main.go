// pipeline orchestrates the full end-to-end flow in a single process:
// crawl -> ocr -> extract. Each stage reuses the existing worker packages
// and shares one DB connection. Fail-fast: a stage error aborts the run.
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"time"

	"skipjd/internal/cmdutil"
	"skipjd/internal/crawler"
	"skipjd/internal/extractor"
	"skipjd/internal/gamejob"
	"skipjd/internal/model"
	"skipjd/internal/ocrworker"
	"skipjd/internal/repository"
)

// runConfig bundles the flag values for run.
type runConfig struct {
	deadline    time.Duration
	httpTimeout time.Duration
	workers     int
	skipCrawl   bool
	skipOCR     bool
	skipExtract bool
	ocrOpts     ocrworker.Options
	extractOpts extractor.Options
}

func main() {
	limit := flag.Int("limit", 50, "max postings per OCR/extract batch")
	offset := flag.Int("offset", 0, "skip the first N postings in OCR/extract batches")
	workers := flag.Int("workers", 3, "concurrent workers per stage (crawl detail, ocr, extract)")
	deadline := flag.Duration("deadline", 0, "max duration for the whole pipeline (0 = no deadline)")
	httpTimeout := flag.Duration("http-timeout", 15*time.Second, "per-attempt HTTP timeout for crawl")
	geminiTimeout := flag.Duration("gemini-timeout", 0, "per-call gemini timeout for ocr/extract (0 = default)")
	minOCRChars := flag.Int("min-ocr-chars", 20, "skip OCR results shorter than this length")
	debugDir := flag.String("debug-dir", "", "directory to retain intermediate files for ocr/extract")
	skipCrawl := flag.Bool("skip-crawl", false, "skip stage 1 (crawl)")
	skipOCR := flag.Bool("skip-ocr", false, "skip stage 2 (ocr)")
	skipExtract := flag.Bool("skip-extract", false, "skip stage 3 (extract)")
	flag.Parse()

	cfg := runConfig{
		deadline:    *deadline,
		httpTimeout: *httpTimeout,
		workers:     *workers,
		skipCrawl:   *skipCrawl,
		skipOCR:     *skipOCR,
		skipExtract: *skipExtract,
		ocrOpts: ocrworker.Options{
			Limit:         *limit,
			Offset:        *offset,
			MinOCRChars:   *minOCRChars,
			DebugDir:      *debugDir,
			Workers:       *workers,
			GeminiTimeout: *geminiTimeout,
		},
		extractOpts: extractor.Options{
			Limit:         *limit,
			Offset:        *offset,
			DebugDir:      *debugDir,
			Workers:       *workers,
			GeminiTimeout: *geminiTimeout,
		},
	}
	if err := run(cfg); err != nil {
		log.Fatal(err)
	}
}

// run is split from main so log.Fatal cannot skip the deferred cancel.
func run(cfg runConfig) error {
	ctx, cancel := cmdutil.SetupContext(cfg.deadline)
	defer cancel()

	db := cmdutil.MustConnectDB()

	if err := db.AutoMigrate(
		&model.CrawlRun{},
		&model.JobPosting{},
		&model.JobPostingDuty{},
		&model.JobPostingBody{},
		&model.JobPostingImage{},
		&model.JobPostingExtraction{},
	); err != nil {
		return fmt.Errorf("failed to migrate db: %w", err)
	}

	repo := repository.NewCrawlerRepository(db)

	if !cfg.skipCrawl {
		start := time.Now()
		if err := runCrawl(ctx, repo, cfg.httpTimeout, cfg.workers); err != nil {
			return fmt.Errorf("[stage=crawl] %w", err)
		}
		log.Printf("[stage=crawl] done in %s", time.Since(start).Round(time.Millisecond))
	}

	if !cfg.skipOCR {
		start := time.Now()
		if err := ocrworker.Run(ctx, repo, cfg.ocrOpts); err != nil {
			return fmt.Errorf("[stage=ocr] %w", err)
		}
		log.Printf("[stage=ocr] done in %s", time.Since(start).Round(time.Millisecond))
	}

	if !cfg.skipExtract {
		start := time.Now()
		if err := extractor.Run(ctx, repo, cfg.extractOpts); err != nil {
			return fmt.Errorf("[stage=extract] %w", err)
		}
		log.Printf("[stage=extract] done in %s", time.Since(start).Round(time.Millisecond))
	}
	return nil
}

func runCrawl(ctx context.Context, repo *repository.CrawlerRepository, httpTimeout time.Duration, detailWorkers int) error {
	scraper, err := gamejob.NewClientScraper(gamejob.WithAttemptTimeout(httpTimeout))
	if err != nil {
		return err
	}
	detailScraper, err := gamejob.NewDetailScraper(nil)
	if err != nil {
		return err
	}
	c, err := crawler.NewCrawler(repo,
		crawler.WithOutput(os.Stdout),
		crawler.WithProgressOutput(os.Stderr),
		crawler.WithCollector(scraper.Scrape),
		crawler.WithDetailCollector(detailScraper.Scrape),
		crawler.WithDetailWorkers(detailWorkers),
	)
	if err != nil {
		return err
	}
	return c.Run(ctx)
}
