package main

import (
	"log"
	"net/http"
	"skipjd/internal/config"
	"skipjd/internal/database"
	"skipjd/internal/handler"
	"skipjd/internal/model"
	"skipjd/internal/repository"
	"skipjd/internal/router"
	"skipjd/internal/service"
	"time"

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
	authService := service.NewAuthService(userRepo, cfg.JWTSecret, cfg.JWTExpire)
	authHandler := handler.NewAuthHandler(authService)

	r := router.Setup(authHandler)
	srv := &http.Server{
		Addr:              ":" + cfg.Port,
		Handler:           r,
		ReadHeaderTimeout: 2 * time.Second,
		ReadTimeout:       5 * time.Second,
		WriteTimeout:      10 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	if err := srv.ListenAndServe(); err != nil {
		log.Fatalf("failed to run server: %v", err)
	}
}
