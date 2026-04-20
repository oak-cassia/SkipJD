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
	}, map[string][]int{
		"jobs/example/123": {1, 3},
	}))

	assertPostingDutyCodes(t, repo, "jobs/example/123", []int{1, 3})

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
	}, map[string][]int{
		"jobs/example/123": {1},
	}))

	var posting model.JobPosting
	require.NoError(t, repo.db.WithContext(ctx).First(&posting).Error)
	assert.Equal(t, "Senior Backend Engineer", posting.Title)
	assert.Equal(t, "https://jobs.example.com/postings/123?updated=true", posting.URL)
	assert.Equal(t, "2026-04-01", posting.ClosingDate)
	assert.Nil(t, posting.MinExperienceYears)
	assert.True(t, posting.FirstSeenAt.Equal(firstSeenAt))
	assert.True(t, posting.LastSeenAt.Equal(lastSeenAt))
	assertPostingDutyCodes(t, repo, "jobs/example/123", []int{1})
}

func TestGetExistingSourceKeysReturnsOnlyMatchedKeys(t *testing.T) {
	repo := NewCrawlerRepository(newCrawlerTestDB(t))
	ctx := context.Background()
	seenAt := time.Date(2026, 3, 31, 10, 0, 0, 0, time.UTC)

	require.NoError(t, repo.UpsertJobPostings(ctx, []model.JobPosting{
		{
			Source:      "browser_agent",
			SourceKey:   "jobs/example/1",
			Title:       "Backend Engineer",
			Company:     "Krafton",
			URL:         "https://jobs.example.com/postings/1",
			ClosingDate: "채용 시 마감",
			FirstSeenAt: seenAt,
			LastSeenAt:  seenAt,
		},
		{
			Source:      "browser_agent",
			SourceKey:   "jobs/example/2",
			Title:       "AI Engineer",
			Company:     "Krafton",
			URL:         "https://jobs.example.com/postings/2",
			ClosingDate: "채용 시 마감",
			FirstSeenAt: seenAt,
			LastSeenAt:  seenAt,
		},
		{
			Source:      "other_source",
			SourceKey:   "jobs/example/3",
			Title:       "Server Engineer",
			Company:     "Other",
			URL:         "https://jobs.example.com/postings/3",
			ClosingDate: "채용 시 마감",
			FirstSeenAt: seenAt,
			LastSeenAt:  seenAt,
		},
	}, map[string][]int{
		"jobs/example/1": {1},
		"jobs/example/2": {3},
		"jobs/example/3": {16},
	}))

	existing, err := repo.GetExistingSourceKeys(ctx, "browser_agent", []string{
		"jobs/example/1",
		"jobs/example/3",
		"jobs/example/missing",
	})
	require.NoError(t, err)

	_, hasFirst := existing["jobs/example/1"]
	_, hasOtherSource := existing["jobs/example/3"]
	_, hasMissing := existing["jobs/example/missing"]
	assert.True(t, hasFirst)
	assert.False(t, hasOtherSource)
	assert.False(t, hasMissing)
}

func newCrawlerTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.CrawlRun{}, &model.JobPosting{}, &model.JobPostingDuty{}))

	return db
}

func TestListJobPostingsByDutyCodesReturnsAnyMatchedPostingsInRecencyOrder(t *testing.T) {
	repo := NewCrawlerRepository(newCrawlerTestDB(t))
	ctx := context.Background()
	baseTime := time.Date(2026, 4, 1, 9, 0, 0, 0, time.UTC)

	require.NoError(t, repo.UpsertJobPostings(ctx, []model.JobPosting{
		{
			Source:      "browser_agent",
			SourceKey:   "jobs/example/1",
			Title:       "Posting 1",
			Company:     "A",
			URL:         "https://jobs.example.com/postings/1",
			ClosingDate: "채용시",
			FirstSeenAt: baseTime,
			LastSeenAt:  baseTime.Add(1 * time.Hour),
		},
		{
			Source:      "browser_agent",
			SourceKey:   "jobs/example/2",
			Title:       "Posting 2",
			Company:     "B",
			URL:         "https://jobs.example.com/postings/2",
			ClosingDate: "채용시",
			FirstSeenAt: baseTime,
			LastSeenAt:  baseTime.Add(3 * time.Hour),
		},
		{
			Source:      "browser_agent",
			SourceKey:   "jobs/example/3",
			Title:       "Posting 3",
			Company:     "C",
			URL:         "https://jobs.example.com/postings/3",
			ClosingDate: "채용시",
			FirstSeenAt: baseTime,
			LastSeenAt:  baseTime.Add(2 * time.Hour),
		},
	}, map[string][]int{
		"jobs/example/1": {1},
		"jobs/example/2": {3, 16},
		"jobs/example/3": {16},
	}))

	postings, err := repo.ListJobPostingsByDutyCodes(ctx, "browser_agent", []int{16, 99})
	require.NoError(t, err)
	require.Len(t, postings, 2)
	assert.Equal(t, []string{"jobs/example/2", "jobs/example/3"}, []string{postings[0].SourceKey, postings[1].SourceKey})
}

func assertPostingDutyCodes(t *testing.T, repo *CrawlerRepository, sourceKey string, want []int) {
	t.Helper()

	var posting model.JobPosting
	require.NoError(t, repo.db.Where("source_key = ?", sourceKey).First(&posting).Error)

	var duties []model.JobPostingDuty
	require.NoError(t, repo.db.
		Where("job_posting_id = ?", posting.ID).
		Order("duty_code ASC").
		Find(&duties).Error)

	got := make([]int, 0, len(duties))
	for _, duty := range duties {
		got = append(got, duty.DutyCode)
	}
	assert.Equal(t, want, got)
}
