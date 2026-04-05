package database

import (
	"fmt"

	"skipjd/internal/config"

	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

func NewGormDB(cfg config.DatabaseConfig) (*gorm.DB, error) {
	tlsMode := "false"
	if cfg.RequireDBTLS {
		tlsMode = "true"
	}

	dsn := fmt.Sprintf(
		"%s:%s@tcp(%s:%s)/%s?charset=utf8mb4&parseTime=True&loc=UTC&tls=%s&timeout=5s&readTimeout=5s&writeTimeout=5s",
		cfg.DBUser,
		cfg.DBPass,
		cfg.DBHost,
		cfg.DBPort,
		cfg.DBName,
		tlsMode,
	)

	return gorm.Open(mysql.Open(dsn), &gorm.Config{})
}
