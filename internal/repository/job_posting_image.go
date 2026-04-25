package repository

import (
	"context"

	"skipjd/internal/model"

	"gorm.io/gorm"
)

// ReplaceJobPostingImages replaces the image queue for a job posting.
// Existing rows are deleted and replaced with the supplied URLs in order.
// If urls is empty, the queue is cleared.
func (r *CrawlerRepository) ReplaceJobPostingImages(ctx context.Context, jobPostingID uint, urls []string) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.
			Where("job_posting_id = ?", jobPostingID).
			Delete(&model.JobPostingImage{}).
			Error; err != nil {
			return err
		}

		if len(urls) == 0 {
			return nil
		}

		rows := make([]model.JobPostingImage, 0, len(urls))
		for index, url := range urls {
			rows = append(rows, model.JobPostingImage{
				JobPostingID: jobPostingID,
				ImageURL:     url,
				OrderIndex:   index,
			})
		}
		return tx.Create(&rows).Error
	})
}

// GetJobPostingIDsBySourceKeys returns a map source_key -> id for the supplied
// keys within the given source. Missing keys are simply absent from the map.
func (r *CrawlerRepository) GetJobPostingIDsBySourceKeys(ctx context.Context, source string, sourceKeys []string) (map[string]uint, error) {
	if len(sourceKeys) == 0 {
		return map[string]uint{}, nil
	}

	rows := make([]struct {
		ID        uint
		SourceKey string
	}, 0, len(sourceKeys))
	if err := r.db.WithContext(ctx).
		Model(&model.JobPosting{}).
		Select("id", "source_key").
		Where("source = ? AND source_key IN ?", source, sourceKeys).
		Find(&rows).
		Error; err != nil {
		return nil, err
	}

	result := make(map[string]uint, len(rows))
	for _, row := range rows {
		result[row.SourceKey] = row.ID
	}
	return result, nil
}
