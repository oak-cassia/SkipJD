// pipeline orchestrates the full end-to-end flow in a single process:
// crawl -> ocr -> extract. Each stage reuses the existing worker packages
// and shares one DB connection. Fail-fast: a stage error aborts the run.
package main

import (
	"context"
	"flag"
	"log"
	"os"
	"time"

	"github.com/joho/godotenv"

	"skipjd/internal/config"
	"skipjd/internal/crawler"
	"skipjd/internal/database"
	"skipjd/internal/extractor"
	"skipjd/internal/gamejob"
	"skipjd/internal/model"
	"skipjd/internal/ocrworker"
	"skipjd/internal/repository"
)

func main() {
	_ = godotenv.Load()

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

	ctx := context.Background()
	if *deadline > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, *deadline)
		defer cancel()
	}

	db, err := database.NewGormDB(config.LoadDatabaseConfig())
	if err != nil {
		log.Fatalf("failed to connect db: %v", err)
	}

	if err := db.AutoMigrate(
		&model.CrawlRun{},
		&model.JobPosting{},
		&model.JobPostingDuty{},
		&model.JobPostingBody{},
		&model.JobPostingImage{},
		&model.JobPostingExtraction{},
	); err != nil {
		log.Fatalf("failed to migrate db: %v", err)
	}

	repo := repository.NewCrawlerRepository(db)

	if !*skipCrawl {
		start := time.Now()
		if err := runCrawl(ctx, repo, *httpTimeout, *workers); err != nil {
			log.Fatalf("[stage=crawl] %v", err)
		}
		log.Printf("[stage=crawl] done in %s", time.Since(start).Round(time.Millisecond))
	}

	if !*skipOCR {
		start := time.Now()
		if err := ocrworker.Run(ctx, repo, ocrworker.Options{
			Limit:         *limit,
			Offset:        *offset,
			MinOCRChars:   *minOCRChars,
			DebugDir:      *debugDir,
			Workers:       *workers,
			GeminiTimeout: *geminiTimeout,
		}); err != nil {
			log.Fatalf("[stage=ocr] %v", err)
		}
		log.Printf("[stage=ocr] done in %s", time.Since(start).Round(time.Millisecond))
	}

	if !*skipExtract {
		start := time.Now()
		if err := extractor.Run(ctx, repo, extractor.Options{
			Limit:         *limit,
			Offset:        *offset,
			DebugDir:      *debugDir,
			Workers:       *workers,
			GeminiTimeout: *geminiTimeout,
		}); err != nil {
			log.Fatalf("[stage=extract] %v", err)
		}
		log.Printf("[stage=extract] done in %s", time.Since(start).Round(time.Millisecond))
	}
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
