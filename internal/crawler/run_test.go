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
	upsertedPostings []model.JobPosting
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

func (r *stubCrawlRunRepository) UpsertJobPostings(ctx context.Context, postings []model.JobPosting) error {
	_ = ctx
	r.upsertedPostings = append([]model.JobPosting(nil), postings...)
	return nil
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

func timePtr(t time.Time) *time.Time {
	return &t
}
