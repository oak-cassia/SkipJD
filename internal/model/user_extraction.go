package model

import "time"

// UserExtraction stores the LLM-derived experience / competency / trait
// profile for a user's resume / self-introduction. Mirrors the shape of
// JobPostingExtraction so user and JD can be matched on the same axes.
//
// SourceHash is a hex SHA256 of the source text fed to the LLM; the worker
// skips the gemini call when the hash for a user is unchanged.
type UserExtraction struct {
	ID         uint   `gorm:"primaryKey"`
	UserID     uint   `gorm:"not null;uniqueIndex"`
	Experience string `gorm:"type:longtext;not null"`
	Competency string `gorm:"type:longtext;not null"`
	Trait      string `gorm:"type:longtext;not null"`
	Model      string `gorm:"size:64;not null"`
	SourceFile string `gorm:"size:512;not null"`
	SourceHash string `gorm:"size:64;not null"`
	CreatedAt  time.Time
	UpdatedAt  time.Time
	User       User `gorm:"constraint:OnDelete:CASCADE;foreignKey:UserID;references:ID"`
}
