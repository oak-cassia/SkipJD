package main

import (
	"log"
	"net/http"
	"skipjd/internal/config"
	"skipjd/internal/database"
	"skipjd/internal/handler"
	"skipjd/internal/middleware"
	"skipjd/internal/model"
	"skipjd/internal/repository"
	"skipjd/internal/router"
	"skipjd/internal/service"
	"strings"
	"time"

	"github.com/joho/godotenv"
)

func main() {
	_ = godotenv.Load()

	cfg := config.Load()
	validateProductionConfig(cfg)

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
	signInLimiter := middleware.NewIPRateLimit(cfg.SignInRateLimit, time.Duration(cfg.SignInRateWindowSecs)*time.Second)

	r := router.Setup(authHandler, authService, signInLimiter)
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

func validateProductionConfig(cfg config.Config) {
	if cfg.AppEnv != "production" {
		return
	}

	if cfg.DBAutoMigrate {
		log.Fatal("DB_AUTO_MIGRATE must be false in production")
	}
	if !cfg.RequireDBTLS {
		log.Fatal("REQUIRE_DB_TLS must be true in production")
	}
	if len(strings.TrimSpace(cfg.JWTSecret)) < 32 {
		log.Fatal("JWT_SECRET must be at least 32 characters in production")
	}
	if cfg.SignInRateLimit <= 0 || cfg.SignInRateWindowSecs <= 0 {
		log.Fatal("SIGNIN_RATE_LIMIT and SIGNIN_RATE_WINDOW_SECS must be greater than zero")
	}
}
