package crawler

import (
	"context"
	"fmt"
	"time"

	"skipjd/internal/gamejob"
)

const dateOnlyFormat = "2006-01-02"

var seoulLocation = time.FixedZone("Asia/Seoul", 9*60*60)

var defaultPreferredCompanies = []string{
	"에피드게임즈",
}

func (c *Crawler) buildCollectOptions(ctx context.Context) (gamejob.CollectOptions, error) {
	lastUpdated, err := c.resolveLastUpdated(ctx)
	if err != nil {
		return gamejob.CollectOptions{}, err
	}

	return gamejob.CollectOptions{
		PreferredCompanies: append([]string(nil), defaultPreferredCompanies...),
		LastUpdated:        dateOnlyInSeoul(lastUpdated),
		TodayDate:          dateOnlyInSeoul(c.now()),
		MaxPages:           gamejob.DefaultMaxPages,
	}, nil
}

func (c *Crawler) resolveLastUpdated(ctx context.Context) (time.Time, error) {
	latestFinishedAt, err := c.crawlerRepository.GetLatestFinishedAtBySource(ctx, appName)
	if err != nil {
		return time.Time{}, fmt.Errorf("get latest finished_at by source: %w", err)
	}
	if latestFinishedAt != nil {
		return *latestFinishedAt, nil
	}

	return c.now().AddDate(0, 0, -14), nil
}

func dateOnlyInSeoul(value time.Time) time.Time {
	local := value.In(seoulLocation)
	return time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, seoulLocation)
}
