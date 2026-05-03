package model

import "time"

type JobPostingExtraction struct {
	ID                  uint      `gorm:"primaryKey"`
	JobPostingID        uint      `gorm:"not null;uniqueIndex"`
	Experience          string    `gorm:"type:longtext;not null"`
	Competency          string    `gorm:"type:longtext;not null"`
	Trait               string    `gorm:"type:longtext;not null"`
	Model               string    `gorm:"size:64;not null"`
	SourceBodyUpdatedAt time.Time `gorm:"not null"`
	CreatedAt           time.Time
	UpdatedAt           time.Time
}
