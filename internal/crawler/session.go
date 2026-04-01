package crawler

import (
	"context"
	"fmt"
	"time"
)

func (c *AICrawler) buildSessionState(ctx context.Context) (map[string]any, error) {
	lastUpdated, err := c.resolveLastUpdated(ctx)
	if err != nil {
		return nil, err
	}

	return map[string]any{
		"target_site": defaultTargetSite,
		"preferred_companies": []string{
			"크래프톤",
		},
		"preferred_positions": []string{
			"게임 서버", "AX",
		},
		"last_updated": lastUpdated.Format(time.RFC3339),
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
