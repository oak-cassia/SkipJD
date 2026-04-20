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
	UpsertJobPostings(ctx context.Context, postings []model.JobPosting, dutyCodesBySourceKey map[string][]int) error
}

func (c *Crawler) persistCrawlResults(
	ctx context.Context,
	postings []model.JobPosting,
	dutyCodesBySourceKey map[string][]int,
	startedAt,
	finishedAt time.Time,
) error {
	if err := c.crawlerRepository.UpsertJobPostings(ctx, postings, dutyCodesBySourceKey); err != nil {
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
