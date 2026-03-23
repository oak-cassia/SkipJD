package main

import (
	"context"
	"log"
	"os"

	"skipjd/internal/crawler"
)

const defaultConfigPath = "configs/crawler.yaml"

func main() {
	ctx := context.Background()

	configPath := os.Getenv("CRAWLER_CONFIG_PATH")
	if configPath == "" {
		configPath = defaultConfigPath
	}

	if err := crawler.Run(ctx, configPath); err != nil {
		log.Fatalf("failed to run crawler: %v", err)
	}
}
