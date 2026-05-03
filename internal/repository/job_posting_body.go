package repository

import (
	"context"
	"errors"

	"skipjd/internal/model"

	"gorm.io/gorm"
)

// UpsertJobPostingBodyHTML stores HTML-extracted body text for a job posting.
// If a body row already exists with Source="ocr", it is preserved (OCR result
// is more informative than re-scraped HTML and we do not want to overwrite it
// on subsequent crawls).
//
// readyForLLM marks whether downstream LLM consumers can treat this body as
// final. The crawler passes false when the same posting also has images
// pending OCR — in that case the OCR worker will flip the flag to true after
// merging the OCR text.
func (r *CrawlerRepository) UpsertJobPostingBodyHTML(ctx context.Context, jobPostingID uint, text string, readyForLLM bool) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var existing model.JobPostingBody
		err := tx.Where("job_posting_id = ?", jobPostingID).Take(&existing).Error
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}

		if errors.Is(err, gorm.ErrRecordNotFound) {
			return tx.Create(&model.JobPostingBody{
				JobPostingID: jobPostingID,
				Text:         text,
				Source:       model.JobPostingBodySourceHTML,
				ReadyForLLM:  readyForLLM,
			}).Error
		}

		if existing.Source == model.JobPostingBodySourceOCR {
			return nil
		}

		return tx.Model(&existing).Updates(map[string]any{
			"text":          text,
			"source":        model.JobPostingBodySourceHTML,
			"ready_for_llm": readyForLLM,
		}).Error
	})
}
