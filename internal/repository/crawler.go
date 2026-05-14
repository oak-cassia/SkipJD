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

type CrawlerRepository struct {
	db *gorm.DB
}

func NewCrawlerRepository(db *gorm.DB) *CrawlerRepository {
	return &CrawlerRepository{db: db}
}

func (r *CrawlerRepository) CreateCrawlRun(ctx context.Context, crawlRun *model.CrawlRun) error {
	return r.db.WithContext(ctx).Create(crawlRun).Error
}

func (r *CrawlerRepository) FinishCrawlRun(ctx context.Context, id uint, finishedAt time.Time) error {
	return r.db.WithContext(ctx).
		Model(&model.CrawlRun{}).
		Where("id = ?", id).
		Update("finished_at", finishedAt).
		Error
}

func (r *CrawlerRepository) GetLatestFinishedAtBySource(ctx context.Context, source string) (*time.Time, error) {
	var crawlRun model.CrawlRun
	err := r.db.WithContext(ctx).
		Select("finished_at").
		Where("source = ?", source).
		Order("finished_at DESC").
		Take(&crawlRun).
		Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	return new(crawlRun.FinishedAt), nil
}

func (r *CrawlerRepository) GetExistingSourceKeys(ctx context.Context, source string, sourceKeys []string) (map[string]struct{}, error) {
	if len(sourceKeys) == 0 {
		return map[string]struct{}{}, nil
	}

	rows := make([]struct {
		SourceKey string
	}, 0, len(sourceKeys))
	if err := r.db.WithContext(ctx).
		Model(&model.JobPosting{}).
		Select("source_key").
		Where("source = ? AND source_key IN ?", source, sourceKeys).
		Find(&rows).
		Error; err != nil {
		return nil, err
	}

	result := make(map[string]struct{}, len(rows))
	for _, row := range rows {
		result[row.SourceKey] = struct{}{}
	}
	return result, nil
}

func (r *CrawlerRepository) UpsertJobPostings(ctx context.Context, postings []model.JobPosting, dutyCodesBySourceKey map[string][]int) error {
	if len(postings) == 0 {
		return nil
	}

	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.
			Clauses(clause.OnConflict{
				Columns: []clause.Column{
					{Name: "source"},
					{Name: "source_key"},
				},
				DoUpdates: clause.AssignmentColumns([]string{
					"title",
					"company",
					"url",
					"closing_date",
					"min_experience_years",
					"last_seen_at",
				}),
			}).
			Create(&postings).
			Error; err != nil {
			return err
		}

		source := postings[0].Source
		sourceKeys := make([]string, 0, len(postings))
		for _, posting := range postings {
			sourceKeys = append(sourceKeys, posting.SourceKey)
		}

		persistedPostings := make([]struct {
			ID        uint
			SourceKey string
		}, 0, len(postings))
		if err := tx.
			Model(&model.JobPosting{}).
			Select("id", "source_key").
			Where("source = ? AND source_key IN ?", source, sourceKeys).
			Find(&persistedPostings).
			Error; err != nil {
			return err
		}

		jobPostingIDs := make([]uint, 0, len(persistedPostings))
		dutyRows := make([]model.JobPostingDuty, 0)
		for _, persistedPosting := range persistedPostings {
			jobPostingIDs = append(jobPostingIDs, persistedPosting.ID)
			for _, dutyCode := range dutyCodesBySourceKey[persistedPosting.SourceKey] {
				dutyRows = append(dutyRows, model.JobPostingDuty{
					JobPostingID: persistedPosting.ID,
					DutyCode:     dutyCode,
				})
			}
		}

		if len(jobPostingIDs) > 0 {
			if err := tx.Where("job_posting_id IN ?", jobPostingIDs).Delete(&model.JobPostingDuty{}).Error; err != nil {
				return err
			}
		}
		if len(dutyRows) > 0 {
			if err := tx.Create(&dutyRows).Error; err != nil {
				return err
			}
		}

		return nil
	})
}

// ListJobPostingsBySource returns all postings for a given source ordered
// by LastSeenAt descending. Used as the fallback candidate list when a user
// has no duty preferences set.
func (r *CrawlerRepository) ListJobPostingsBySource(ctx context.Context, source string) ([]model.JobPosting, error) {
	postings := make([]model.JobPosting, 0)
	if err := r.db.WithContext(ctx).
		Where("source = ?", source).
		Order("last_seen_at DESC").
		Order("id DESC").
		Find(&postings).
		Error; err != nil {
		return nil, fmt.Errorf("list job postings by source: %w", err)
	}
	return postings, nil
}

func (r *CrawlerRepository) ListJobPostingsByDutyCodes(ctx context.Context, source string, dutyCodes []int) ([]model.JobPosting, error) {
	if len(dutyCodes) == 0 {
		return []model.JobPosting{}, nil
	}

	postings := make([]model.JobPosting, 0)
	if err := r.db.WithContext(ctx).
		Model(&model.JobPosting{}).
		Distinct("job_postings.*").
		Joins("JOIN job_posting_duties ON job_posting_duties.job_posting_id = job_postings.id").
		Where("job_postings.source = ? AND job_posting_duties.duty_code IN ?", source, dutyCodes).
		Order("job_postings.last_seen_at DESC").
		Order("job_postings.id DESC").
		Find(&postings).
		Error; err != nil {
		return nil, fmt.Errorf("list job postings by duty codes: %w", err)
	}

	return postings, nil
}
