package main

import (
	"log"
	"skipjd/internal/config"
	"skipjd/internal/database"
	"skipjd/internal/handler"
	"skipjd/internal/model"
	"skipjd/internal/repository"
	"skipjd/internal/router"
	"skipjd/internal/service"

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

	userRepo := repository.NewUserRepository(db)
	jwtProvider := service.NewJWTProvider(cfg.JWTSecret, cfg.JWTExpire)
	authService := service.NewAuthService(userRepo, jwtProvider)
	authHandler := handler.NewAuthHandler(authService)

	r := router.Setup(authHandler)
	if err := r.Run(":" + cfg.Port); err != nil {
		log.Fatalf("failed to run server: %v", err)
	}
}
