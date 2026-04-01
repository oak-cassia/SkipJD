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
	GetExistingSourceKeys(ctx context.Context, source string, sourceKeys []string) (map[string]struct{}, error)
	UpsertJobPostings(ctx context.Context, postings []model.JobPosting) error
}

func (c *AICrawler) findNewPostings(ctx context.Context, postings []model.JobPosting) ([]model.JobPosting, error) {
	sourceKeys := collectSourceKeys(postings)
	existingSourceKeys, err := c.crawlerRepository.GetExistingSourceKeys(ctx, appName, sourceKeys)
	if err != nil {
		return nil, fmt.Errorf("get existing source keys: %w", err)
	}

	return filterNewPostings(postings, existingSourceKeys), nil
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

func (c *AICrawler) notifyNewPostings(ctx context.Context, finishedAt time.Time, newPostings []model.JobPosting) {
	if len(newPostings) == 0 || c.mailer == nil {
		return
	}
	if err := c.mailer.SendDigest(ctx, finishedAt, newPostings); err != nil {
		_, _ = fmt.Fprintf(c.out, "digest email failed: %v\n", err)
		return
	}

	_, _ = fmt.Fprintf(c.out, "digest email sent: %d new postings\n", len(newPostings))
}

func collectSourceKeys(postings []model.JobPosting) []string {
	// 값 자체는 필요 없어서 빈 구조체(struct{})를 사용
	seen := make(map[string]struct{}, len(postings))
	keys := make([]string, 0, len(postings))

	for _, posting := range postings {
		key := posting.SourceKey
		if key == "" {
			continue
		}
		if _, ok := seen[key]; ok {
			continue
		}

		// 처음 본 키만 set과 결과 목록에 추가
		seen[key] = struct{}{}
		keys = append(keys, key)
	}

	return keys
}

func filterNewPostings(postings []model.JobPosting, existingSourceKeys map[string]struct{}) []model.JobPosting {
	if len(postings) == 0 {
		return nil
	}

	newPostings := make([]model.JobPosting, 0, len(postings))
	for _, posting := range postings {
		if _, exists := existingSourceKeys[posting.SourceKey]; exists {
			continue
		}
		newPostings = append(newPostings, posting)
	}
	return newPostings
}
