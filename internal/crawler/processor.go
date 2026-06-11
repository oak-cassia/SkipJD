package crawler

import (
	"context"
	"fmt"
	"time"

	"skipjd/internal/gamejob"
	"skipjd/internal/model"
)

type crawlRunRepository interface {
	CreateCrawlRun(ctx context.Context, crawlRun *model.CrawlRun) error
	GetLatestFinishedAtBySource(ctx context.Context, source string) (*time.Time, error)
	UpsertJobPostings(ctx context.Context, postings []model.JobPosting, dutyCodesBySourceKey map[string][]int) error
	GetJobPostingIDsBySourceKeys(ctx context.Context, source string, sourceKeys []string) (map[string]uint, error)
	UpsertJobPostingBodyHTML(ctx context.Context, jobPostingID uint, text string, readyForLLM bool) error
	ReplaceJobPostingImages(ctx context.Context, jobPostingID uint, urls []string) error
}

func (c *Crawler) persistCrawlResults(
	ctx context.Context,
	postings []model.JobPosting,
	dutyCodesBySourceKey map[string][]int,
	detailBySourceKey map[string]gamejob.DetailContent,
	startedAt,
	finishedAt time.Time,
) error {
	if err := c.crawlerRepository.UpsertJobPostings(ctx, postings, dutyCodesBySourceKey); err != nil {
		return fmt.Errorf("upsert job postings: %w", err)
	}

	if err := c.persistPostingDetails(ctx, postings, detailBySourceKey); err != nil {
		return err
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

func (c *Crawler) persistPostingDetails(
	ctx context.Context,
	postings []model.JobPosting,
	detailBySourceKey map[string]gamejob.DetailContent,
) error {
	if len(detailBySourceKey) == 0 {
		return nil
	}

	sourceKeys := make([]string, 0, len(postings))
	for i := range postings {
		if _, exists := detailBySourceKey[postings[i].SourceKey]; exists {
			sourceKeys = append(sourceKeys, postings[i].SourceKey)
		}
	}
	if len(sourceKeys) == 0 {
		return nil
	}

	idsBySourceKey, err := c.crawlerRepository.GetJobPostingIDsBySourceKeys(ctx, appName, sourceKeys)
	if err != nil {
		return fmt.Errorf("lookup job posting ids: %w", err)
	}

	for _, sourceKey := range sourceKeys {
		id, ok := idsBySourceKey[sourceKey]
		if !ok {
			continue
		}
		detail := detailBySourceKey[sourceKey]

		hasImages := len(detail.ImageURLs) > 0
		if detail.TextContent != "" {
			// HTML-only postings are immediately LLM-ready; postings with
			// pending images stay un-ready until the OCR worker finalizes
			// the body.
			readyForLLM := !hasImages
			if err := c.crawlerRepository.UpsertJobPostingBodyHTML(ctx, id, detail.TextContent, readyForLLM); err != nil {
				return fmt.Errorf("upsert body for %s: %w", sourceKey, err)
			}
		}
		if hasImages {
			if err := c.crawlerRepository.ReplaceJobPostingImages(ctx, id, detail.ImageURLs); err != nil {
				return fmt.Errorf("replace images for %s: %w", sourceKey, err)
			}
		}
	}

	return nil
}
