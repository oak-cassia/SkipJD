package repository

import (
	"context"
	"errors"
	"fmt"

	"skipjd/internal/model"

	"gorm.io/gorm"
)

type UserExtractionRepository struct {
	db *gorm.DB
}

func NewUserExtractionRepository(db *gorm.DB) *UserExtractionRepository {
	return &UserExtractionRepository{db: db}
}

// ListUserIDsWithExtraction returns the set of user IDs that have a
// UserExtraction row, ordered ascending. Used by cmd/notify when run in
// "all users" mode (no --user-id flag).
func (r *UserExtractionRepository) ListUserIDsWithExtraction(ctx context.Context) ([]uint, error) {
	rows := make([]struct{ UserID uint }, 0)
	if err := r.db.WithContext(ctx).
		Model(&model.UserExtraction{}).
		Select("user_id").
		Order("user_id ASC").
		Find(&rows).
		Error; err != nil {
		return nil, fmt.Errorf("list user ids with extraction: %w", err)
	}
	ids := make([]uint, 0, len(rows))
	for _, row := range rows {
		ids = append(ids, row.UserID)
	}
	return ids, nil
}

// GetByUserID returns the UserExtraction row for the given user, or
// (nil, nil) if no row exists yet.
func (r *UserExtractionRepository) GetByUserID(ctx context.Context, userID uint) (*model.UserExtraction, error) {
	var row model.UserExtraction
	err := r.db.WithContext(ctx).
		Where("user_id = ?", userID).
		Take(&row).
		Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		// "no extraction yet" is a valid state callers branch on, not an error.
		return nil, nil //nolint:nilnil // not-found contract is (nil, nil)
	}
	if err != nil {
		return nil, fmt.Errorf("get user extraction: %w", err)
	}
	return &row, nil
}

// GetSourceHash returns the stored SourceHash for the given user, or "" if
// no extraction row exists yet.
func (r *UserExtractionRepository) GetSourceHash(ctx context.Context, userID uint) (string, error) {
	var row model.UserExtraction
	err := r.db.WithContext(ctx).
		Select("source_hash").
		Where("user_id = ?", userID).
		Take(&row).
		Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("get user extraction hash: %w", err)
	}
	return row.SourceHash, nil
}

// UpsertUserExtraction inserts or updates a UserExtraction row for the
// given user. The experience / competency / trait arguments are JSON strings
// already encoded by the caller (use extractor.EncodeArray).
func (r *UserExtractionRepository) UpsertUserExtraction(
	ctx context.Context,
	userID uint,
	experience, competency, trait, modelName, sourceFile, sourceHash string,
) error {
	if userID == 0 {
		return errors.New("upsert user extraction: userID must be > 0")
	}

	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var existing model.UserExtraction
		err := tx.Where("user_id = ?", userID).Take(&existing).Error
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}

		if errors.Is(err, gorm.ErrRecordNotFound) {
			return tx.Create(&model.UserExtraction{
				UserID:     userID,
				Experience: experience,
				Competency: competency,
				Trait:      trait,
				Model:      modelName,
				SourceFile: sourceFile,
				SourceHash: sourceHash,
			}).Error
		}

		return tx.Model(&existing).Updates(map[string]any{
			"experience":  experience,
			"competency":  competency,
			"trait":       trait,
			"model":       modelName,
			"source_file": sourceFile,
			"source_hash": sourceHash,
		}).Error
	})
}
