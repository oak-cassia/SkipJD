package main

import (
	"flag"
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

	ctx, cancel := cmdutil.SetupContext(*deadline)
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
		log.Fatalf("failed to migrate db: %v", err)
	}

	crawlerRepository := repository.NewCrawlerRepository(db)

	scraper, err := gamejob.NewClientScraper(gamejob.WithAttemptTimeout(*httpTimeout))
	if err != nil {
		log.Fatalf("failed to create client scraper: %v", err)
	}
	detailScraper, err := gamejob.NewDetailScraper(nil)
	if err != nil {
		log.Fatalf("failed to create detail scraper: %v", err)
	}

	c, err := crawler.NewCrawler(crawlerRepository,
		crawler.WithOutput(os.Stdout),
		crawler.WithProgressOutput(os.Stderr),
		crawler.WithCollector(scraper.Scrape),
		crawler.WithDetailCollector(detailScraper.Scrape),
		crawler.WithDetailWorkers(*detailWorkers),
	)
	if err != nil {
		log.Fatalf("failed to create crawler: %v", err)
	}

	if err := c.Run(ctx); err != nil {
		log.Fatalf("failed to run crawler: %v", err)
	}
}
