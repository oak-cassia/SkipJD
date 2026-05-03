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
	ReadyForLLM  bool   `gorm:"column:ready_for_llm;not null;default:false;index"`
	CreatedAt    time.Time
	UpdatedAt    time.Time
}
