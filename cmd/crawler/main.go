package main

import (
	"context"
	"log"

	"github.com/joho/godotenv"

	"skipjd/internal/config"
	"skipjd/internal/crawler"
	"skipjd/internal/database"
	"skipjd/internal/model"
	"skipjd/internal/repository"
)

func main() {
	_ = godotenv.Load()

	ctx := context.Background()

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
	); err != nil {
		log.Fatalf("failed to migrate db: %v", err)
	}

	crawlerRepository := repository.NewCrawlerRepository(db)

	if err := crawler.Run(ctx, crawlerRepository); err != nil {
		log.Fatalf("failed to run crawler: %v", err)
	}
}
