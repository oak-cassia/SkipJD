package model

type JobPostingDuty struct {
	JobPostingID uint       `gorm:"not null;uniqueIndex:uq_job_posting_duties_job_posting_id_duty_code;index:idx_job_posting_duties_duty_code_job_posting_id,priority:2"`
	DutyCode     int        `gorm:"not null;uniqueIndex:uq_job_posting_duties_job_posting_id_duty_code;index:idx_job_posting_duties_duty_code_job_posting_id,priority:1"`
	JobPosting   JobPosting `gorm:"constraint:OnDelete:CASCADE;foreignKey:JobPostingID;references:ID"`
}
