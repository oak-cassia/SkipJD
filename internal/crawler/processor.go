package crawler

import (
	"context"
	"fmt"
	"time"

	"skipjd/internal/model"
)

type crawlRunRepository interface {
	CreateCrawlRun(ctx context.Context, crawlRun *model.CrawlRun) error
	GetLatestFinishedAtBySource(ctx context.Context, source string) (*time.Time, error)
	UpsertJobPostings(ctx context.Context, postings []model.JobPosting) error
}

func (c *AICrawler) persistCrawlResults(ctx context.Context, postings []model.JobPosting, startedAt, finishedAt time.Time) error {
	if err := c.crawlerRepository.UpsertJobPostings(ctx, postings); err != nil {
		return fmt.Errorf("upsert job postings: %w", err)
	}
	if err := c.crawlerRepository.CreateCrawlRun(ctx, &model.CrawlRun{
		Source:     appName,
		StartedAt:  startedAt,
		FinishedAt: finishedAt,
	}); err != nil {
		return fmt.Errorf("create crawl run: %w", err)
	}

	return nil
}
