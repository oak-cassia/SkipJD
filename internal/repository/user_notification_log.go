package repository

import (
	"context"
	"errors"
	"fmt"
	"time"

	"skipjd/internal/model"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type UserNotificationLogRepository struct {
	db *gorm.DB
}

func NewUserNotificationLogRepository(db *gorm.DB) *UserNotificationLogRepository {
	return &UserNotificationLogRepository{db: db}
}

// GetSentPostingIDs returns the set of JobPosting IDs already sent to the
// given user. Used to filter candidates before scoring so already-mailed
// postings never reach the mailer twice.
func (r *UserNotificationLogRepository) GetSentPostingIDs(ctx context.Context, userID uint) (map[uint]struct{}, error) {
	rows := make([]struct{ JobPostingID uint }, 0)
	if err := r.db.WithContext(ctx).
		Model(&model.UserNotificationLog{}).
		Select("job_posting_id").
		Where("user_id = ?", userID).
		Find(&rows).
		Error; err != nil {
		return nil, fmt.Errorf("get sent posting ids: %w", err)
	}
	out := make(map[uint]struct{}, len(rows))
	for _, row := range rows {
		out[row.JobPostingID] = struct{}{}
	}
	return out, nil
}

// Record inserts notification log rows for all postingIDs at sentAt.
// Uses OnConflict DoNothing so re-running the same notify against the same
// posting set is safe (idempotent at the persistence layer in addition to
// the candidate filter).
func (r *UserNotificationLogRepository) Record(ctx context.Context, userID uint, postingIDs []uint, sentAt time.Time) error {
	if userID == 0 {
		return errors.New("record notifications: userID must be > 0")
	}
	if len(postingIDs) == 0 {
		return nil
	}
	rows := make([]model.UserNotificationLog, 0, len(postingIDs))
	for _, id := range postingIDs {
		rows = append(rows, model.UserNotificationLog{
			UserID:       userID,
			JobPostingID: id,
			SentAt:       sentAt,
		})
	}
	return r.db.WithContext(ctx).
		Clauses(clause.OnConflict{DoNothing: true}).
		Create(&rows).
		Error
}
