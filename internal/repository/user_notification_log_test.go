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

func newNotificationLogTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.JobPosting{}, &model.UserNotificationLog{}))
	return db
}

func TestRecordAndGetSentPostingIDs(t *testing.T) {
	db := newNotificationLogTestDB(t)
	repo := NewUserNotificationLogRepository(db)
	ctx := context.Background()
	now := time.Date(2026, 5, 14, 9, 0, 0, 0, time.UTC)

	require.NoError(t, repo.Record(ctx, 1, []uint{10, 20, 30}, now))

	got, err := repo.GetSentPostingIDs(ctx, 1)
	require.NoError(t, err)
	assert.ElementsMatch(t, []uint{10, 20, 30}, keysOf(got))
}

func TestGetSentPostingIDsIsolatesPerUser(t *testing.T) {
	db := newNotificationLogTestDB(t)
	repo := NewUserNotificationLogRepository(db)
	ctx := context.Background()
	now := time.Now()

	require.NoError(t, repo.Record(ctx, 1, []uint{10, 20}, now))
	require.NoError(t, repo.Record(ctx, 2, []uint{30}, now))

	user1, err := repo.GetSentPostingIDs(ctx, 1)
	require.NoError(t, err)
	user2, err := repo.GetSentPostingIDs(ctx, 2)
	require.NoError(t, err)

	assert.ElementsMatch(t, []uint{10, 20}, keysOf(user1))
	assert.ElementsMatch(t, []uint{30}, keysOf(user2))
}

func TestRecordIsIdempotentOnSamePair(t *testing.T) {
	db := newNotificationLogTestDB(t)
	repo := NewUserNotificationLogRepository(db)
	ctx := context.Background()
	now := time.Now()

	require.NoError(t, repo.Record(ctx, 1, []uint{10, 20}, now))
	require.NoError(t, repo.Record(ctx, 1, []uint{10, 20, 30}, now.Add(time.Hour)),
		"OnConflict DoNothing must allow re-insert of existing pair")

	got, err := repo.GetSentPostingIDs(ctx, 1)
	require.NoError(t, err)
	assert.ElementsMatch(t, []uint{10, 20, 30}, keysOf(got))
}

func TestRecordEmptyPostingIDsIsNoop(t *testing.T) {
	db := newNotificationLogTestDB(t)
	repo := NewUserNotificationLogRepository(db)
	require.NoError(t, repo.Record(context.Background(), 1, nil, time.Now()))
}

func keysOf(m map[uint]struct{}) []uint {
	out := make([]uint, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
