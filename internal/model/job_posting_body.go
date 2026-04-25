package model

import "time"

const (
	JobPostingBodySourceHTML = "html"
	JobPostingBodySourceOCR  = "ocr"
)

type JobPostingBody struct {
	ID           uint   `gorm:"primaryKey"`
	JobPostingID uint   `gorm:"not null;uniqueIndex"`
	Text         string `gorm:"type:longtext;not null"`
	Source       string `gorm:"size:16;not null"`
	CreatedAt    time.Time
	UpdatedAt    time.Time
}
