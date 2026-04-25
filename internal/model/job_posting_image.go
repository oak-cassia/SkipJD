package model

type JobPostingImage struct {
	ID           uint   `gorm:"primaryKey"`
	JobPostingID uint   `gorm:"not null;index"`
	ImageURL     string `gorm:"type:text;not null"`
	OrderIndex   int    `gorm:"not null"`
}
