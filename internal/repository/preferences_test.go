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

func TestEnsureUserIsIdempotent(t *testing.T) {
	repo := NewPreferencesRepository(newPreferencesTestDB(t))
	ctx := context.Background()

	first, err := repo.EnsureUser(ctx, 1)
	require.NoError(t, err)
	require.Equal(t, uint(1), first.ID)
	require.False(t, first.CreatedAt.IsZero())

	second, err := repo.EnsureUser(ctx, 1)
	require.NoError(t, err)
	assert.Equal(t, first.ID, second.ID)
	assert.True(t, first.CreatedAt.Equal(second.CreatedAt), "CreatedAt should be stable across EnsureUser calls")
}

func TestEnsureUserRejectsZeroID(t *testing.T) {
	repo := NewPreferencesRepository(newPreferencesTestDB(t))

	_, err := repo.EnsureUser(context.Background(), 0)
	assert.Error(t, err)
}

func TestReplaceUserDutyPreferencesReplacesAllRows(t *testing.T) {
	repo := NewPreferencesRepository(newPreferencesTestDB(t))
	ctx := context.Background()
	_, err := repo.EnsureUser(ctx, 1)
	require.NoError(t, err)

	require.NoError(t, repo.ReplaceUserDutyPreferences(ctx, 1, []int{1, 3}))
	codes, err := repo.GetUserDutyCodes(ctx, 1)
	require.NoError(t, err)
	assert.Equal(t, []int{1, 3}, codes)

	require.NoError(t, repo.ReplaceUserDutyPreferences(ctx, 1, []int{16, 16, 3}))
	codes, err = repo.GetUserDutyCodes(ctx, 1)
	require.NoError(t, err)
	assert.Equal(t, []int{3, 16}, codes, "duplicates removed, old codes replaced")
}

func TestReplaceUserDutyPreferencesWithEmptyClearsAll(t *testing.T) {
	repo := NewPreferencesRepository(newPreferencesTestDB(t))
	ctx := context.Background()
	_, err := repo.EnsureUser(ctx, 1)
	require.NoError(t, err)

	require.NoError(t, repo.ReplaceUserDutyPreferences(ctx, 1, []int{1, 3}))
	require.NoError(t, repo.ReplaceUserDutyPreferences(ctx, 1, nil))

	codes, err := repo.GetUserDutyCodes(ctx, 1)
	require.NoError(t, err)
	assert.Empty(t, codes)
}

func TestReplaceUserCompanyPreferencesNormalizesAndDeduplicates(t *testing.T) {
	repo := NewPreferencesRepository(newPreferencesTestDB(t))
	ctx := context.Background()
	_, err := repo.EnsureUser(ctx, 1)
	require.NoError(t, err)

	require.NoError(t, repo.ReplaceUserCompanyPreferences(ctx, 1, []string{
		"㈜넵튠",
		"넵튠",
		"라인게임즈㈜",
		"",
		"  ",
	}))

	names, err := repo.GetUserCompanyNames(ctx, 1)
	require.NoError(t, err)
	assert.Equal(t, []string{"넵튠", "라인게임즈"}, names)
}

func TestReplaceUserCompanyPreferencesIsolatesByUser(t *testing.T) {
	repo := NewPreferencesRepository(newPreferencesTestDB(t))
	ctx := context.Background()
	_, err := repo.EnsureUser(ctx, 1)
	require.NoError(t, err)
	_, err = repo.EnsureUser(ctx, 2)
	require.NoError(t, err)

	require.NoError(t, repo.ReplaceUserCompanyPreferences(ctx, 1, []string{"넵튠"}))
	require.NoError(t, repo.ReplaceUserCompanyPreferences(ctx, 2, []string{"크래프톤"}))
	require.NoError(t, repo.ReplaceUserCompanyPreferences(ctx, 1, []string{"크래프톤"}))

	names1, err := repo.GetUserCompanyNames(ctx, 1)
	require.NoError(t, err)
	assert.Equal(t, []string{"크래프톤"}, names1)

	names2, err := repo.GetUserCompanyNames(ctx, 2)
	require.NoError(t, err)
	assert.Equal(t, []string{"크래프톤"}, names2, "replacing user 1 must not touch user 2")
}

func TestListDistinctCompanyNamesFromJobPostings(t *testing.T) {
	db := newPreferencesTestDB(t)
	repo := NewPreferencesRepository(db)
	crawlerRepo := NewCrawlerRepository(db)
	ctx := context.Background()
	seenAt := time.Date(2026, 4, 1, 9, 0, 0, 0, time.UTC)

	require.NoError(t, crawlerRepo.UpsertJobPostings(ctx, []model.JobPosting{
		{Source: "browser_agent", SourceKey: "k/1", Title: "T1", Company: "넵튠", URL: "u1", FirstSeenAt: seenAt, LastSeenAt: seenAt},
		{Source: "browser_agent", SourceKey: "k/2", Title: "T2", Company: "넵튠", URL: "u2", FirstSeenAt: seenAt, LastSeenAt: seenAt},
		{Source: "browser_agent", SourceKey: "k/3", Title: "T3", Company: "㈜크래프톤", URL: "u3", FirstSeenAt: seenAt, LastSeenAt: seenAt},
		{Source: "browser_agent", SourceKey: "k/4", Title: "T4", Company: "크래프톤", URL: "u4", FirstSeenAt: seenAt, LastSeenAt: seenAt},
	}, map[string][]int{
		"k/1": {1}, "k/2": {1}, "k/3": {3}, "k/4": {3},
	}))

	names, err := repo.ListDistinctCompanyNames(ctx)
	require.NoError(t, err)
	assert.Equal(t, []string{"넵튠", "크래프톤"}, names)
}

func newPreferencesTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(
		&model.JobPosting{},
		&model.JobPostingDuty{},
		&model.User{},
		&model.UserDutyPreference{},
		&model.UserCompanyPreference{},
	))

	return db
}
