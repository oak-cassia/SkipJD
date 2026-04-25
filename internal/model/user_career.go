package model

type UserCareer struct {
	UserID          uint `gorm:"primaryKey;autoIncrement:false"`
	ExperienceYears *int `gorm:"index:idx_user_careers_experience_years"`
	User            User `gorm:"constraint:OnDelete:CASCADE;foreignKey:UserID;references:ID"`
}
