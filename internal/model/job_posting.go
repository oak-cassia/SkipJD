package model

import "time"

type JobPosting struct {
	ID                 uint   `gorm:"primaryKey"`
	Source             string `gorm:"size:64;not null;uniqueIndex:uq_job_postings_source_source_key"`
	SourceKey          string `gorm:"size:255;not null;uniqueIndex:uq_job_postings_source_source_key"`
	Title              string `gorm:"size:255;not null"`
	Company            string `gorm:"size:255;not null"`
	URL                string `gorm:"type:text;not null"`
	ClosingDate        string `gorm:"size:64"`
	MinExperienceYears *int
	FirstSeenAt        time.Time `gorm:"not null"`
	LastSeenAt         time.Time `gorm:"not null"`
}
