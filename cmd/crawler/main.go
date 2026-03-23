package main

import (
	"context"
	"log"
	"os"

	"github.com/joho/godotenv"

	"skipjd/internal/config"
	"skipjd/internal/crawler"
	"skipjd/internal/database"
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
	_ = db

	configPath := os.Getenv("CRAWLER_CONFIG_PATH")
	if configPath == "" {
		configPath = defaultConfigPath
	}

	if err := crawler.Run(ctx, configPath); err != nil {
		log.Fatalf("failed to run crawler: %v", err)
	}
}
