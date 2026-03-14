package main

import (
	"log"

	"github.com/joho/godotenv"

	"skipjd/internal/config"
	"skipjd/internal/database"
	"skipjd/internal/router"
)

func main() {
	_ = godotenv.Load()

	cfg := config.Load()

	db, err := database.NewGormDB(cfg)
	if err != nil {
		log.Fatalf("failed to connect db: %v", err)
	}

	r := router.Setup(db)
	if err := r.Run(":" + cfg.Port); err != nil {
		log.Fatalf("failed to run server: %v", err)
	}
}
