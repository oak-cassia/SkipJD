package model

type UserCompanyPreference struct {
	UserID      uint   `gorm:"not null;uniqueIndex:uq_user_company_preferences_user_id_company_name"`
	CompanyName string `gorm:"size:255;not null;uniqueIndex:uq_user_company_preferences_user_id_company_name;index:idx_user_company_preferences_company_name"`
	User        User   `gorm:"constraint:OnDelete:CASCADE;foreignKey:UserID;references:ID"`
}
