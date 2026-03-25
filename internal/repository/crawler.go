package repository

import (
	"context"
	"errors"
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

func (r *CrawlerRepository) UpsertJobPostings(ctx context.Context, postings []model.JobPosting) error {
	if len(postings) == 0 {
		return nil
	}

	return r.db.WithContext(ctx).
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
		Error
}
