package main

import (
	"log"
	"skipjd/internal/config"
	"skipjd/internal/database"
	"skipjd/internal/model"
	"skipjd/internal/router"

	"github.com/joho/godotenv"
)

func main() {
	_ = godotenv.Load()

	cfg := config.Load()

	db, err := database.NewGormDB(cfg)
	if err != nil {
		log.Fatalf("failed to connect db: %v", err)
	}

	if cfg.DBAutoMigrate {
		if err := db.AutoMigrate(
			&model.User{},
			//&model.AlertSetting{},
			//&model.JobPosting{},
			//&model.SentJobAlert{},
		); err != nil {
			log.Fatal(err)
		}
	}

	r := router.Setup(db)
	if err := r.Run(":" + cfg.Port); err != nil {
		log.Fatalf("failed to run server: %v", err)
	}
}
