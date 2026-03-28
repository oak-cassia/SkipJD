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

func TestUpsertJobPostingsUpdatesExistingPosting(t *testing.T) {
	repo := NewCrawlerRepository(newCrawlerTestDB(t))
	ctx := context.Background()
	firstSeenAt := time.Date(2026, 3, 1, 9, 0, 0, 0, time.UTC)
	lastSeenAt := firstSeenAt.Add(24 * time.Hour)
	experienceYears := 3

	require.NoError(t, repo.UpsertJobPostings(ctx, []model.JobPosting{
		{
			Source:             "browser_agent",
			SourceKey:          "jobs/example/123",
			Title:              "Backend Engineer",
			Company:            "Krafton",
			URL:                "https://jobs.example.com/postings/123",
			ClosingDate:        "채용 시 마감",
			MinExperienceYears: &experienceYears,
			FirstSeenAt:        firstSeenAt,
			LastSeenAt:         firstSeenAt,
		},
	}))

	require.NoError(t, repo.UpsertJobPostings(ctx, []model.JobPosting{
		{
			Source:      "browser_agent",
			SourceKey:   "jobs/example/123",
			Title:       "Senior Backend Engineer",
			Company:     "Krafton",
			URL:         "https://jobs.example.com/postings/123?updated=true",
			ClosingDate: "2026-04-01",
			FirstSeenAt: lastSeenAt,
			LastSeenAt:  lastSeenAt,
		},
	}))

	var posting model.JobPosting
	require.NoError(t, repo.db.WithContext(ctx).First(&posting).Error)
	assert.Equal(t, "Senior Backend Engineer", posting.Title)
	assert.Equal(t, "https://jobs.example.com/postings/123?updated=true", posting.URL)
	assert.Equal(t, "2026-04-01", posting.ClosingDate)
	assert.Nil(t, posting.MinExperienceYears)
	assert.True(t, posting.FirstSeenAt.Equal(firstSeenAt))
	assert.True(t, posting.LastSeenAt.Equal(lastSeenAt))
}

func newCrawlerTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.CrawlRun{}, &model.JobPosting{}))

	return db
}
