package model

import "time"

// UserNotificationLog records that a job posting has been sent to a user in
// a digest mail. The composite primary key (UserID, JobPostingID) makes
// "send once and never resend" the natural semantic — inserts with the
// same pair are skipped via OnConflict.
type UserNotificationLog struct {
	UserID       uint       `gorm:"primaryKey;autoIncrement:false"`
	JobPostingID uint       `gorm:"primaryKey;autoIncrement:false"`
	SentAt       time.Time  `gorm:"not null"`
	User         User       `gorm:"constraint:OnDelete:CASCADE;foreignKey:UserID;references:ID"`
	JobPosting   JobPosting `gorm:"constraint:OnDelete:CASCADE;foreignKey:JobPostingID;references:ID"`
}
