package main

import (
	"context"
	"log"
	"os"

	"github.com/joho/godotenv"

	"skipjd/internal/config"
	"skipjd/internal/crawler"
	"skipjd/internal/database"
	"skipjd/internal/repository"
)

const defaultConfigPath = "configs/crawler.yaml"

func main() {
	_ = godotenv.Load()

	ctx := context.Background()
	cfg := config.Load()

	db, err := database.NewGormDB(cfg)
	if err != nil {
		log.Fatalf("failed to connect db: %v", err)
	}

	crawlerRepository := repository.NewCrawlerRepository(db)

	configPath := os.Getenv("CRAWLER_CONFIG_PATH")
	if configPath == "" {
		configPath = defaultConfigPath
	}

	if err := crawler.Run(ctx, configPath, crawlerRepository); err != nil {
		log.Fatalf("failed to run crawler: %v", err)
	}
}
