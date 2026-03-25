package crawler

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"skipjd/internal/model"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildSessionStateUsesLatestFinishedAt(t *testing.T) {
	repo := &stubCrawlRunRepository{
		latestFinishedAt: timePtr(time.Date(2026, 3, 20, 10, 30, 0, 0, time.UTC)),
	}
	crawler := &AICrawler{
		crawlerRepository: repo,
		now: func() time.Time {
			return time.Date(2026, 3, 25, 8, 0, 0, 0, time.UTC)
		},
	}

	state, err := crawler.buildSessionState(context.Background())
	require.NoError(t, err)

	assert.Equal(t, defaultTargetSite, state["target_site"])
	assert.Equal(t, []string{"크래프톤"}, state["preferred_companies"])
	assert.Equal(t, []string{"게임 서버", "AX"}, state["preferred_positions"])
	assert.Equal(t, "2026-03-20T10:30:00Z", state["last_updated"])
	assert.Equal(t, appName, repo.lastSource)
}

func TestBuildSessionStateFallsBackToTwoWeeksBeforeNow(t *testing.T) {
	now := time.Date(2026, 3, 25, 8, 0, 0, 0, time.FixedZone("KST", 9*60*60))
	crawler := &AICrawler{
		crawlerRepository: &stubCrawlRunRepository{},
		now: func() time.Time {
			return now
		},
	}

	state, err := crawler.buildSessionState(context.Background())
	require.NoError(t, err)

	assert.Equal(t, "2026-03-11T08:00:00+09:00", state["last_updated"])
}

func TestCrawlerConfigInstructionIncludesLastUpdatedConstraint(t *testing.T) {
	configPath := filepath.Join("..", "..", "configs", "crawler.yaml")

	data, err := os.ReadFile(configPath)
	require.NoError(t, err)

	content := string(data)
	assert.Contains(t, content, "{last_updated}")
	assert.Contains(t, content, "{target_site}")
	assert.Contains(t, content, "{preferred_companies}")
	assert.Contains(t, content, "{preferred_positions}")
}

type stubCrawlRunRepository struct {
	latestFinishedAt *time.Time
	lastSource       string
}

func (r *stubCrawlRunRepository) CreateCrawlRun(ctx context.Context, crawlRun *model.CrawlRun) error {
	_ = ctx
	_ = crawlRun
	return nil
}

func (r *stubCrawlRunRepository) GetLatestFinishedAtBySource(ctx context.Context, source string) (*time.Time, error) {
	_ = ctx
	r.lastSource = source
	return r.latestFinishedAt, nil
}

func timePtr(t time.Time) *time.Time {
	return &t
}
