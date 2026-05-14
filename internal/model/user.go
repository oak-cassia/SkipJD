package model

import (
	"time"

	"gorm.io/gorm"
)

type User struct {
	ID        uint   `gorm:"primaryKey"`
	Email     string `gorm:"size:255;not null;uniqueIndex:idx_users_email"`
	Name      string `gorm:"size:100;not null"`
	IsActive  bool   `gorm:"not null;default:1"`
	CreatedAt time.Time
	UpdatedAt time.Time
	DeletedAt gorm.DeletedAt `gorm:"index:idx_users_deleted_at"`
}
