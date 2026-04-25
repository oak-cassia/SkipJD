package repository

import (
	"context"
	"testing"

	"skipjd/internal/model"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func newPostingBodyTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.JobPostingBody{}, &model.JobPostingImage{}))
	return db
}

func TestUpsertJobPostingBodyHTMLInsertsWhenAbsent(t *testing.T) {
	repo := NewCrawlerRepository(newPostingBodyTestDB(t))
	ctx := context.Background()

	require.NoError(t, repo.UpsertJobPostingBodyHTML(ctx, 1, "본문 텍스트"))

	var stored model.JobPostingBody
	require.NoError(t, repo.db.WithContext(ctx).First(&stored, "job_posting_id = ?", 1).Error)
	assert.Equal(t, "본문 텍스트", stored.Text)
	assert.Equal(t, model.JobPostingBodySourceHTML, stored.Source)
}

func TestUpsertJobPostingBodyHTMLUpdatesExistingHTMLRow(t *testing.T) {
	repo := NewCrawlerRepository(newPostingBodyTestDB(t))
	ctx := context.Background()

	require.NoError(t, repo.UpsertJobPostingBodyHTML(ctx, 1, "이전 본문"))
	require.NoError(t, repo.UpsertJobPostingBodyHTML(ctx, 1, "새 본문"))

	var stored model.JobPostingBody
	require.NoError(t, repo.db.WithContext(ctx).First(&stored, "job_posting_id = ?", 1).Error)
	assert.Equal(t, "새 본문", stored.Text)
	assert.Equal(t, model.JobPostingBodySourceHTML, stored.Source)
}

func TestUpsertJobPostingBodyHTMLPreservesExistingOCRRow(t *testing.T) {
	db := newPostingBodyTestDB(t)
	repo := NewCrawlerRepository(db)
	ctx := context.Background()

	require.NoError(t, db.Create(&model.JobPostingBody{
		JobPostingID: 1,
		Text:         "OCR로 추출된 본문",
		Source:       model.JobPostingBodySourceOCR,
	}).Error)

	require.NoError(t, repo.UpsertJobPostingBodyHTML(ctx, 1, "재크롤된 HTML 본문"))

	var stored model.JobPostingBody
	require.NoError(t, db.WithContext(ctx).First(&stored, "job_posting_id = ?", 1).Error)
	assert.Equal(t, "OCR로 추출된 본문", stored.Text)
	assert.Equal(t, model.JobPostingBodySourceOCR, stored.Source)
}

func TestReplaceJobPostingImagesInsertsInOrder(t *testing.T) {
	repo := NewCrawlerRepository(newPostingBodyTestDB(t))
	ctx := context.Background()

	require.NoError(t, repo.ReplaceJobPostingImages(ctx, 1, []string{"a.png", "b.png", "c.png"}))

	var rows []model.JobPostingImage
	require.NoError(t, repo.db.WithContext(ctx).Order("order_index").Find(&rows, "job_posting_id = ?", 1).Error)
	require.Len(t, rows, 3)
	for i, row := range rows {
		assert.Equal(t, i, row.OrderIndex)
	}
	assert.Equal(t, "a.png", rows[0].ImageURL)
}

func TestReplaceJobPostingImagesDeletesPreviousRows(t *testing.T) {
	repo := NewCrawlerRepository(newPostingBodyTestDB(t))
	ctx := context.Background()

	require.NoError(t, repo.ReplaceJobPostingImages(ctx, 1, []string{"old.png"}))
	require.NoError(t, repo.ReplaceJobPostingImages(ctx, 1, []string{"new1.png", "new2.png"}))

	var rows []model.JobPostingImage
	require.NoError(t, repo.db.WithContext(ctx).Order("order_index").Find(&rows, "job_posting_id = ?", 1).Error)
	require.Len(t, rows, 2)
	assert.Equal(t, "new1.png", rows[0].ImageURL)
	assert.Equal(t, "new2.png", rows[1].ImageURL)
}

func TestReplaceJobPostingImagesEmptyClearsQueue(t *testing.T) {
	repo := NewCrawlerRepository(newPostingBodyTestDB(t))
	ctx := context.Background()

	require.NoError(t, repo.ReplaceJobPostingImages(ctx, 1, []string{"a.png"}))
	require.NoError(t, repo.ReplaceJobPostingImages(ctx, 1, nil))

	var count int64
	require.NoError(t, repo.db.WithContext(ctx).Model(&model.JobPostingImage{}).Where("job_posting_id = ?", 1).Count(&count).Error)
	assert.Equal(t, int64(0), count)
}
