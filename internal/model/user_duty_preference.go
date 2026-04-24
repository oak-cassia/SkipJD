package model

type UserDutyPreference struct {
	UserID   uint `gorm:"not null;uniqueIndex:uq_user_duty_preferences_user_id_duty_code;index:idx_user_duty_preferences_duty_code_user_id,priority:2"`
	DutyCode int  `gorm:"not null;uniqueIndex:uq_user_duty_preferences_user_id_duty_code;index:idx_user_duty_preferences_duty_code_user_id,priority:1"`
	User     User `gorm:"constraint:OnDelete:CASCADE;foreignKey:UserID;references:ID"`
}
