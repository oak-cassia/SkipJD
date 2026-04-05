package crawler

import (
	"context"
	"fmt"
	"time"
)

const sessionDateFormat = "2006-01-02"

var seoulLocation = time.FixedZone("Asia/Seoul", 9*60*60)

func (c *AICrawler) buildSessionState(ctx context.Context) (map[string]any, error) {
	lastUpdated, err := c.resolveLastUpdated(ctx)
	if err != nil {
		return nil, err
	}

	return map[string]any{
		"preferred_companies": []string{
			"크래프톤",
		},
		"last_updated": lastUpdated.In(seoulLocation).Format(sessionDateFormat),
	}, nil
}

func (c *AICrawler) resolveLastUpdated(ctx context.Context) (time.Time, error) {
	latestFinishedAt, err := c.crawlerRepository.GetLatestFinishedAtBySource(ctx, appName)
	if err != nil {
		return time.Time{}, fmt.Errorf("get latest finished_at by source: %w", err)
	}
	if latestFinishedAt != nil {
		return *latestFinishedAt, nil
	}

	return c.now().AddDate(0, 0, -14), nil
}
