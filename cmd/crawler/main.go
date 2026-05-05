package main

import (
	"context"
	"flag"
	"log"
	"time"

	"github.com/joho/godotenv"

	"skipjd/internal/config"
	"skipjd/internal/crawler"
	"skipjd/internal/database"
	"skipjd/internal/model"
	"skipjd/internal/repository"
)

func main() {
	_ = godotenv.Load()

	detailWorkers := flag.Int("detail-workers", 5, "Number of concurrent detail worker routines")
	httpTimeout := flag.Duration("http-timeout", 15*time.Second, "Timeout per HTTP attempt")
	deadline := flag.Duration("deadline", 0, "Max duration for the crawler batch (0 = no deadline)")
	flag.Parse()

	ctx := context.Background()
	if *deadline > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, *deadline)
		defer cancel()
	}

	cfg := config.LoadDatabaseConfig()

	db, err := database.NewGormDB(cfg)
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

	crawlerRepository := repository.NewCrawlerRepository(db)

	opts := crawler.RunOptions{
		DetailWorkers:  *detailWorkers,
		AttemptTimeout: *httpTimeout,
	}

	if err := crawler.Run(ctx, crawlerRepository, opts); err != nil {
		log.Fatalf("failed to run crawler: %v", err)
	}
}
