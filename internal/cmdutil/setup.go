// Package cmdutil collapses the env-load + context-deadline + DB-connect
// preamble shared by every binary under cmd/. Repositories stay as-is —
// callers wrap the returned *gorm.DB in whichever repository.NewXxx they
// need.
package cmdutil

import (
	"context"
	"log"
	"time"

	"github.com/joho/godotenv"
	"gorm.io/gorm"

	"skipjd/internal/config"
	"skipjd/internal/database"
)

// SetupContext loads .env (best-effort, errors ignored) and builds a
// context with an optional deadline. Callers must always defer the
// returned cancel; for deadline == 0 it is a no-op.
func SetupContext(deadline time.Duration) (context.Context, context.CancelFunc) {
	_ = godotenv.Load()
	if deadline > 0 {
		return context.WithTimeout(context.Background(), deadline)
	}
	return context.Background(), func() {}
}

// MustConnectDB opens the configured GORM DB or log.Fatals on failure.
// Returned *gorm.DB is the input to repository.NewXxxRepository(db).
func MustConnectDB() *gorm.DB {
	db, err := database.NewGormDB(config.LoadDatabaseConfig())
	if err != nil {
		log.Fatalf("failed to connect db: %v", err)
	}
	return db
}
