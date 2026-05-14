package repository

import (
	"context"
	"errors"
	"fmt"
	"time"

	"skipjd/internal/model"

	"gorm.io/gorm"
)

// PendingBody is the row shape returned by FetchPendingExtractions.
type PendingBody struct {
	JobPostingID  uint
	Text          string
	BodyUpdatedAt time.Time
}

// FetchPendingExtractions returns body rows that are flagged ready_for_llm
// and either have no extraction yet, or have a body update newer than the
// extraction's recorded source_body_updated_at. Ordered by body updated_at
// descending.
func (r *CrawlerRepository) FetchPendingExtractions(ctx context.Context, limit, offset int) ([]PendingBody, error) {
	rows := make([]PendingBody, 0, limit)
	err := r.db.WithContext(ctx).
		Table("job_posting_bodies").
		Select(
			"job_posting_bodies.job_posting_id AS job_posting_id, "+
				"job_posting_bodies.text AS text, "+
				"job_posting_bodies.updated_at AS body_updated_at",
		).
		Joins(
			"LEFT JOIN job_posting_extractions "+
				"ON job_posting_extractions.job_posting_id = job_posting_bodies.job_posting_id",
		).
		Where("job_posting_bodies.ready_for_llm = ?", true).
		Where(
			"job_posting_extractions.id IS NULL " +
				"OR job_posting_extractions.source_body_updated_at < job_posting_bodies.updated_at",
		).
		Order("job_posting_bodies.updated_at DESC").
		Offset(offset).
		Limit(limit).
		Scan(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("fetch pending extractions: %w", err)
	}
	return rows, nil
}

// GetExtractionsByPostingIDs returns JobPostingExtraction rows keyed by
// JobPostingID. Postings without an extraction row are simply absent from
// the map (caller filters or scores them as zero).
func (r *CrawlerRepository) GetExtractionsByPostingIDs(ctx context.Context, postingIDs []uint) (map[uint]model.JobPostingExtraction, error) {
	if len(postingIDs) == 0 {
		return map[uint]model.JobPostingExtraction{}, nil
	}
	rows := make([]model.JobPostingExtraction, 0, len(postingIDs))
	if err := r.db.WithContext(ctx).
		Where("job_posting_id IN ?", postingIDs).
		Find(&rows).
		Error; err != nil {
		return nil, fmt.Errorf("get extractions by posting ids: %w", err)
	}

	out := make(map[uint]model.JobPostingExtraction, len(rows))
	for _, row := range rows {
		out[row.JobPostingID] = row
	}
	return out, nil
}

// UpsertJobPostingExtraction inserts or updates a JobPostingExtraction row
// for the given posting. The experience/competency/trait arguments are JSON
// strings already encoded by the caller; sourceBodyUpdatedAt is the source
// body's updated_at, used by FetchPendingExtractions to detect staleness.
func (r *CrawlerRepository) UpsertJobPostingExtraction(
	ctx context.Context,
	jobPostingID uint,
	experience, competency, trait, modelName string,
	sourceBodyUpdatedAt time.Time,
) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var existing model.JobPostingExtraction
		err := tx.Where("job_posting_id = ?", jobPostingID).Take(&existing).Error
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}

		if errors.Is(err, gorm.ErrRecordNotFound) {
			return tx.Create(&model.JobPostingExtraction{
				JobPostingID:        jobPostingID,
				Experience:          experience,
				Competency:          competency,
				Trait:               trait,
				Model:               modelName,
				SourceBodyUpdatedAt: sourceBodyUpdatedAt,
			}).Error
		}

		return tx.Model(&existing).Updates(map[string]any{
			"experience":             experience,
			"competency":             competency,
			"trait":                  trait,
			"model":                  modelName,
			"source_body_updated_at": sourceBodyUpdatedAt,
		}).Error
	})
}
