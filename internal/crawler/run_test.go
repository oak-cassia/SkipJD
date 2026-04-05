package crawler

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
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

	assert.Equal(t, map[string]any{
		"preferred_companies": []string{"크래프톤"},
		"last_updated":        "2026-03-20",
	}, state)
	assert.Equal(t, appName, repo.lastSource)
}

func TestBuildSessionStateConvertsLatestFinishedAtToKSTDate(t *testing.T) {
	repo := &stubCrawlRunRepository{
		latestFinishedAt: timePtr(time.Date(2026, 3, 20, 23, 30, 0, 0, time.UTC)),
	}
	crawler := &AICrawler{
		crawlerRepository: repo,
		now: func() time.Time {
			return time.Date(2026, 3, 25, 8, 0, 0, 0, time.UTC)
		},
	}

	state, err := crawler.buildSessionState(context.Background())
	require.NoError(t, err)

	assert.Equal(t, "2026-03-21", state["last_updated"])
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

	assert.Equal(t, "2026-03-11", state["last_updated"])
}

func TestCrawlerConfigInstructionIncludesLastUpdatedConstraint(t *testing.T) {
	configPath := filepath.Join("..", "..", "configs", "crawler.yaml")

	data, err := os.ReadFile(configPath)
	require.NoError(t, err)

	content := string(data)
	assert.Contains(t, content, "{last_updated}")
	assert.Contains(t, content, "{preferred_companies}")
	assert.Contains(t, content, "https://www.gamejob.co.kr/Recruit/joblist?menucode=duty&duty=1")
	assert.Contains(t, content, "https://www.gamejob.co.kr/Recruit/joblist?menucode=duty&duty=3")
	assert.Contains(t, content, "https://www.gamejob.co.kr/Recruit/joblist?menucode=duty&duty=16")
	assert.Contains(t, content, "수정일순")
	assert.Contains(t, content, "collect postings whose listing modified date is on or after {last_updated}")
	assert.Contains(t, content, "listing modified date is earlier than {last_updated}")
	assert.Contains(t, content, "stop scanning that page and move to the next listing page")
}

type stubCrawlRunRepository struct {
	latestFinishedAt   *time.Time
	lastSource         string
	upsertedPostings   []model.JobPosting
	createdCrawlRun    *model.CrawlRun
	existingSourceKeys map[string]struct{}
}

func (r *stubCrawlRunRepository) CreateCrawlRun(ctx context.Context, crawlRun *model.CrawlRun) error {
	_ = ctx
	copied := *crawlRun
	r.createdCrawlRun = &copied
	return nil
}

func (r *stubCrawlRunRepository) GetLatestFinishedAtBySource(ctx context.Context, source string) (*time.Time, error) {
	_ = ctx
	r.lastSource = source
	return r.latestFinishedAt, nil
}

func (r *stubCrawlRunRepository) GetExistingSourceKeys(ctx context.Context, source string, sourceKeys []string) (map[string]struct{}, error) {
	_ = ctx
	r.lastSource = source

	result := make(map[string]struct{})
	for _, sourceKey := range sourceKeys {
		if _, exists := r.existingSourceKeys[sourceKey]; !exists {
			continue
		}
		result[sourceKey] = struct{}{}
	}
	return result, nil
}

func (r *stubCrawlRunRepository) UpsertJobPostings(ctx context.Context, postings []model.JobPosting) error {
	_ = ctx
	r.upsertedPostings = append([]model.JobPosting(nil), postings...)
	return nil
}

type stubMailer struct {
	sendCalls int
	sendErr   error
	lastRunAt time.Time
	postings  []model.JobPosting
}

func (m *stubMailer) SendDigest(ctx context.Context, runAt time.Time, postings []model.JobPosting) error {
	_ = ctx
	m.sendCalls++
	m.lastRunAt = runAt
	m.postings = append([]model.JobPosting(nil), postings...)
	return m.sendErr
}

func TestParseCollectedPostingsBuildsJobPostingModels(t *testing.T) {
	seenAt := time.Date(2026, 3, 25, 9, 0, 0, 0, time.UTC)
	output := `{"postings":[{"title":"Backend Engineer","company":"Krafton","url":"https://jobs.example.com/postings/123#details","closing_date":"채용 시 마감","min_experience_years":3},{"title":"AI Engineer","company":"Krafton","url":"https://jobs.example.com/postings/456","closing_date":"2026-04-01"}]}`

	postings, err := parseCollectedPostings(output, seenAt)
	require.NoError(t, err)
	require.Len(t, postings, 2)

	assert.Equal(t, appName, postings[0].Source)
	assert.Equal(t, "https://jobs.example.com/postings/123", postings[0].SourceKey)
	assert.Equal(t, "Backend Engineer", postings[0].Title)
	assert.Equal(t, "Krafton", postings[0].Company)
	assert.Equal(t, "https://jobs.example.com/postings/123#details", postings[0].URL)
	assert.Equal(t, "채용 시 마감", postings[0].ClosingDate)
	require.NotNil(t, postings[0].MinExperienceYears)
	assert.Equal(t, 3, *postings[0].MinExperienceYears)
	assert.True(t, postings[0].FirstSeenAt.Equal(seenAt))
	assert.True(t, postings[0].LastSeenAt.Equal(seenAt))

	assert.Equal(t, "https://jobs.example.com/postings/456", postings[1].SourceKey)
	assert.Nil(t, postings[1].MinExperienceYears)
}

func TestParseCollectedPostingsRejectsMissingRequiredFields(t *testing.T) {
	_, err := parseCollectedPostings(`{"postings":[{"company":"Krafton","url":"https://jobs.example.com/postings/123","closing_date":"채용 시 마감"}]}`, time.Now())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "title is required")
}

func TestFilterNewPostingsByExistingSourceKeys(t *testing.T) {
	postings := []model.JobPosting{
		{SourceKey: "jobs/example/1", Title: "A"},
		{SourceKey: "jobs/example/2", Title: "B"},
		{SourceKey: "jobs/example/3", Title: "C"},
	}

	newPostings := filterNewPostings(postings, map[string]struct{}{
		"jobs/example/2": {},
	})

	require.Len(t, newPostings, 2)
	assert.Equal(t, "jobs/example/1", newPostings[0].SourceKey)
	assert.Equal(t, "jobs/example/3", newPostings[1].SourceKey)
}

func TestPersistenceFlowSkipsDigestWhenNoNewPostings(t *testing.T) {
	now := time.Date(2026, 3, 31, 9, 0, 0, 0, time.UTC)
	postings := []model.JobPosting{
		{
			Source:      appName,
			SourceKey:   "jobs/example/1",
			Title:       "A",
			Company:     "Krafton",
			ClosingDate: "채용 시 마감",
			URL:         "https://jobs.example.com/1",
		},
	}
	repo := &stubCrawlRunRepository{
		existingSourceKeys: map[string]struct{}{
			"jobs/example/1": {},
		},
	}
	mailer := &stubMailer{}
	crawler := &AICrawler{
		out:               &bytes.Buffer{},
		crawlerRepository: repo,
		mailer:            mailer,
	}

	ctx := context.Background()
	newPostings, err := crawler.findNewPostings(ctx, postings)
	require.NoError(t, err)
	require.Len(t, newPostings, 0)

	err = crawler.persistCrawlResults(ctx, postings, now.Add(-5*time.Minute), now)
	require.NoError(t, err)
	crawler.notifyNewPostings(ctx, now, newPostings)

	assert.Equal(t, 0, mailer.sendCalls)
	require.Len(t, repo.upsertedPostings, 1)
	require.NotNil(t, repo.createdCrawlRun)
}

func TestNotifyNewPostingsLogsMailError(t *testing.T) {
	now := time.Date(2026, 3, 31, 9, 0, 0, 0, time.UTC)
	postings := []model.JobPosting{
		{
			Source:      appName,
			SourceKey:   "jobs/example/1",
			Title:       "A",
			Company:     "Krafton",
			ClosingDate: "채용 시 마감",
			URL:         "https://jobs.example.com/1",
		},
	}
	repo := &stubCrawlRunRepository{}
	mailer := &stubMailer{
		sendErr: errors.New("smtp down"),
	}
	output := &bytes.Buffer{}
	crawler := &AICrawler{
		out:               output,
		crawlerRepository: repo,
		mailer:            mailer,
	}

	ctx := context.Background()
	newPostings, err := crawler.findNewPostings(ctx, postings)
	require.NoError(t, err)
	require.Len(t, newPostings, 1)

	err = crawler.persistCrawlResults(ctx, postings, now.Add(-5*time.Minute), now)
	require.NoError(t, err)
	crawler.notifyNewPostings(ctx, now, newPostings)

	assert.Equal(t, 1, mailer.sendCalls)
	require.Len(t, repo.upsertedPostings, 1)
	require.NotNil(t, repo.createdCrawlRun)
	assert.True(t, strings.Contains(output.String(), "digest email failed"))
}

func timePtr(t time.Time) *time.Time {
	return &t
}
