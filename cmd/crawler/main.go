package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"time"

	"skipjd/internal/cmdutil"
	"skipjd/internal/crawler"
	"skipjd/internal/gamejob"
	"skipjd/internal/model"
	"skipjd/internal/repository"
)

func main() {
	detailWorkers := flag.Int("detail-workers", 5, "Number of concurrent detail worker routines")
	httpTimeout := flag.Duration("http-timeout", 15*time.Second, "Timeout per HTTP attempt")
	deadline := flag.Duration("deadline", 0, "Max duration for the crawler batch (0 = no deadline)")
	flag.Parse()

	if err := run(*deadline, *httpTimeout, *detailWorkers); err != nil {
		log.Fatal(err)
	}
}

// run is split from main so log.Fatal cannot skip the deferred cancel.
func run(deadline, httpTimeout time.Duration, detailWorkers int) error {
	ctx, cancel := cmdutil.SetupContext(deadline)
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

	crawlerRepository := repository.NewCrawlerRepository(db)

	scraper, err := gamejob.NewClientScraper(gamejob.WithAttemptTimeout(httpTimeout))
	if err != nil {
		return fmt.Errorf("failed to create client scraper: %w", err)
	}
	detailScraper, err := gamejob.NewDetailScraper(nil)
	if err != nil {
		return fmt.Errorf("failed to create detail scraper: %w", err)
	}

	c, err := crawler.NewCrawler(crawlerRepository,
		crawler.WithOutput(os.Stdout),
		crawler.WithProgressOutput(os.Stderr),
		crawler.WithCollector(scraper.Scrape),
		crawler.WithDetailCollector(detailScraper.Scrape),
		crawler.WithDetailWorkers(detailWorkers),
	)
	if err != nil {
		return fmt.Errorf("failed to create crawler: %w", err)
	}

	if err := c.Run(ctx); err != nil {
		return fmt.Errorf("failed to run crawler: %w", err)
	}
	return nil
}
