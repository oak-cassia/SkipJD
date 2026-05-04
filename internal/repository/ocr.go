package repository

import (
	"context"
	"errors"
	"fmt"
	"time"

	"skipjd/internal/model"

	"gorm.io/gorm"
)

// FetchPendingOCRPostingIDs returns posting IDs that have at least one
// JobPostingImage but no JobPostingBody with source="ocr" yet, ordered by
// JobPosting.LastSeenAt descending.
func (r *CrawlerRepository) FetchPendingOCRPostingIDs(ctx context.Context, limit, offset int) ([]uint, error) {
	rows := make([]struct {
		JobPostingID uint
		LastSeenAt   time.Time
	}, 0, limit)
	err := r.db.WithContext(ctx).
		Table("job_posting_images").
		Select(
			"DISTINCT job_posting_images.job_posting_id AS job_posting_id, " +
				"job_postings.last_seen_at AS last_seen_at",
		).
		Joins("JOIN job_postings ON job_postings.id = job_posting_images.job_posting_id").
		Joins(
			"LEFT JOIN job_posting_bodies "+
				"ON job_posting_bodies.job_posting_id = job_posting_images.job_posting_id "+
				"AND job_posting_bodies.source = ?",
			model.JobPostingBodySourceOCR,
		).
		Where("job_posting_bodies.id IS NULL").
		Order("job_postings.last_seen_at DESC").
		Offset(offset).
		Limit(limit).
		Scan(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("fetch pending ocr postings: %w", err)
	}

	ids := make([]uint, 0, len(rows))
	for _, row := range rows {
		ids = append(ids, row.JobPostingID)
	}
	return ids, nil
}

// FetchImagesForPosting returns all JobPostingImage rows for the given
// posting ID ordered by OrderIndex ascending.
func (r *CrawlerRepository) FetchImagesForPosting(ctx context.Context, jobPostingID uint) ([]model.JobPostingImage, error) {
	images := make([]model.JobPostingImage, 0)
	err := r.db.WithContext(ctx).
		Where("job_posting_id = ?", jobPostingID).
		Order("order_index ASC").
		Find(&images).Error
	if err != nil {
		return nil, fmt.Errorf("fetch images for posting %d: %w", jobPostingID, err)
	}
	return images, nil
}

// UpsertJobPostingBodyOCR persists OCR-extracted text for a posting.
//
// If no body row exists, a new one is inserted with source="ocr" and
// ReadyForLLM=true. If an HTML-sourced body already exists, the OCR text is
// appended with a "[OCR]" separator (preserves crawler-extracted HTML for
// traceability). If an OCR body already exists, the text is replaced.
func (r *CrawlerRepository) UpsertJobPostingBodyOCR(ctx context.Context, jobPostingID uint, ocrText string) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var existing model.JobPostingBody
		err := tx.Where("job_posting_id = ?", jobPostingID).Take(&existing).Error
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}

		if errors.Is(err, gorm.ErrRecordNotFound) {
			return tx.Create(&model.JobPostingBody{
				JobPostingID: jobPostingID,
				Text:         ocrText,
				Source:       model.JobPostingBodySourceOCR,
				ReadyForLLM:  true,
			}).Error
		}

		merged := ocrText
		if existing.Source == model.JobPostingBodySourceHTML {
			merged = existing.Text + "\n\n[OCR]\n" + ocrText
		}
		return tx.Model(&existing).Updates(map[string]any{
			"text":          merged,
			"source":        model.JobPostingBodySourceOCR,
			"ready_for_llm": true,
		}).Error
	})
}
