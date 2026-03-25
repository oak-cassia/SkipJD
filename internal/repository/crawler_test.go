package repository

import (
	"context"
	"testing"
	"time"

	"skipjd/internal/model"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestGetLatestFinishedAtBySourceReturnsNewestRunForSource(t *testing.T) {
	repo := NewCrawlerRepository(newCrawlerTestDB(t))
	ctx := context.Background()

	older := time.Date(2026, 3, 1, 9, 0, 0, 0, time.FixedZone("KST", 9*60*60))
	newer := older.Add(24 * time.Hour)

	require.NoError(t, repo.CreateCrawlRun(ctx, &model.CrawlRun{
		Source:     "browser_agent",
		StartedAt:  older.Add(-10 * time.Minute),
		FinishedAt: older,
	}))
	require.NoError(t, repo.CreateCrawlRun(ctx, &model.CrawlRun{
		Source:     "browser_agent",
		StartedAt:  newer.Add(-10 * time.Minute),
		FinishedAt: newer,
	}))
	require.NoError(t, repo.CreateCrawlRun(ctx, &model.CrawlRun{
		Source:     "other_source",
		StartedAt:  newer.Add(-10 * time.Minute),
		FinishedAt: newer.Add(24 * time.Hour),
	}))

	finishedAt, err := repo.GetLatestFinishedAtBySource(ctx, "browser_agent")
	require.NoError(t, err)
	require.NotNil(t, finishedAt)
	assert.True(t, finishedAt.Equal(newer))
}

func TestGetLatestFinishedAtBySourceReturnsNilWhenMissing(t *testing.T) {
	repo := NewCrawlerRepository(newCrawlerTestDB(t))

	finishedAt, err := repo.GetLatestFinishedAtBySource(context.Background(), "browser_agent")
	require.NoError(t, err)
	assert.Nil(t, finishedAt)
}

func newCrawlerTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.CrawlRun{}))

	return db
}
