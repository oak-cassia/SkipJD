package model

import "time"

type CrawlRun struct {
	ID         uint      `gorm:"primaryKey"`
	Source     string    `gorm:"size:64;not null"`
	StartedAt  time.Time `gorm:"not null"`
	FinishedAt *time.Time
}
