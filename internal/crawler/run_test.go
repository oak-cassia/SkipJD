package crawler

import (
	"bytes"
	"context"
	"io"
	"testing"
	"time"

	"skipjd/internal/gamejob"
	"skipjd/internal/model"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildCollectOptionsUsesLatestFinishedAt(t *testing.T) {
	repo := &stubCrawlRunRepository{
		latestFinishedAt: timePtr(time.Date(2026, 3, 20, 10, 30, 0, 0, time.UTC)),
	}
	crawler := newTestCrawler(t, repo, noopCollect, func() time.Time {
		return time.Date(2026, 3, 25, 8, 0, 0, 0, time.UTC)
	}, nil, nil)

	opts, err := crawler.buildCollectOptions(context.Background())
	require.NoError(t, err)

	assert.Equal(t, []string{"에피드게임즈"}, opts.PreferredCompanies)
	assert.Equal(t, "2026-03-20", opts.LastUpdated.In(seoulLocation).Format(dateOnlyFormat))
	assert.Equal(t, "2026-03-25", opts.TodayDate.In(seoulLocation).Format(dateOnlyFormat))
	assert.Equal(t, gamejob.DefaultMaxPages, opts.MaxPages)
	assert.Equal(t, appName, repo.lastSource)
}

func TestBuildCollectOptionsConvertsLatestFinishedAtToKSTDate(t *testing.T) {
	repo := &stubCrawlRunRepository{
		latestFinishedAt: timePtr(time.Date(2026, 3, 20, 23, 30, 0, 0, time.UTC)),
	}
	crawler := newTestCrawler(t, repo, noopCollect, func() time.Time {
		return time.Date(2026, 3, 25, 8, 0, 0, 0, time.UTC)
	}, nil, nil)

	opts, err := crawler.buildCollectOptions(context.Background())
	require.NoError(t, err)

	assert.Equal(t, "2026-03-21", opts.LastUpdated.In(seoulLocation).Format(dateOnlyFormat))
	assert.Equal(t, "2026-03-25", opts.TodayDate.In(seoulLocation).Format(dateOnlyFormat))
}

func TestBuildCollectOptionsFallsBackToTwoWeeksBeforeNow(t *testing.T) {
	now := time.Date(2026, 3, 25, 8, 0, 0, 0, time.FixedZone("KST", 9*60*60))
	crawler := newTestCrawler(t, &stubCrawlRunRepository{}, noopCollect, func() time.Time {
		return now
	}, nil, nil)

	opts, err := crawler.buildCollectOptions(context.Background())
	require.NoError(t, err)

	assert.Equal(t, "2026-03-11", opts.LastUpdated.In(seoulLocation).Format(dateOnlyFormat))
	assert.Equal(t, "2026-03-25", opts.TodayDate.In(seoulLocation).Format(dateOnlyFormat))
}

type stubCrawlRunRepository struct {
	latestFinishedAt *time.Time
	lastSource       string
	upsertedPostings []model.JobPosting
	createdCrawlRun  *model.CrawlRun
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

func (r *stubCrawlRunRepository) UpsertJobPostings(ctx context.Context, postings []model.JobPosting) error {
	_ = ctx
	r.upsertedPostings = append([]model.JobPosting(nil), postings...)
	return nil
}

type stubCollector struct {
	postings []model.JobPosting
	lastOpts gamejob.CollectOptions
	calls    int
}

func (c *stubCollector) Collect(_ context.Context, opts gamejob.CollectOptions) ([]model.JobPosting, error) {
	c.calls++
	c.lastOpts = opts
	return append([]model.JobPosting(nil), c.postings...), nil
}

func TestRunCollectsWithScriptAndPersistsResults(t *testing.T) {
	minYears := 3
	postings := []model.JobPosting{
		{
			Source:             appName,
			SourceKey:          "https://www.gamejob.co.kr/Recruit/GI_Read/View?GI_No=275868",
			Title:              "Server Engineer",
			Company:            "에피드게임즈",
			ClosingDate:        "채용시",
			URL:                "https://www.gamejob.co.kr/Recruit/GI_Read/View?GI_No=275868",
			MinExperienceYears: &minYears,
		},
	}
	repo := &stubCrawlRunRepository{
		latestFinishedAt: timePtr(time.Date(2026, 3, 20, 23, 30, 0, 0, time.UTC)),
	}
	collector := &stubCollector{postings: postings}
	var out bytes.Buffer
	var progress bytes.Buffer

	crawler := newTestCrawler(t, repo, collector.Collect, func() time.Time {
		return time.Date(2026, 3, 25, 8, 0, 0, 0, time.UTC)
	}, &out, &progress)

	err := crawler.Run(context.Background())
	require.NoError(t, err)

	assert.Equal(t, 1, collector.calls)
	assert.Equal(t, []string{"에피드게임즈"}, collector.lastOpts.PreferredCompanies)
	assert.Equal(t, "2026-03-21", collector.lastOpts.LastUpdated.In(seoulLocation).Format(dateOnlyFormat))
	assert.Equal(t, "2026-03-25", collector.lastOpts.TodayDate.In(seoulLocation).Format(dateOnlyFormat))
	assert.Equal(t, gamejob.DefaultMaxPages, collector.lastOpts.MaxPages)
	assert.JSONEq(t, `{"postings":[{"title":"Server Engineer","company":"에피드게임즈","url":"https://www.gamejob.co.kr/Recruit/GI_Read/View?GI_No=275868","closing_date":"채용시","min_experience_years":3}]}`, out.String())
	assert.Equal(t, postings, repo.upsertedPostings)
	require.NotNil(t, repo.createdCrawlRun)
	assert.Equal(t, appName, repo.createdCrawlRun.Source)
	assert.Contains(t, progress.String(), "collect_options preferred_companies=에피드게임즈 last_updated=2026-03-21 today_date=2026-03-25 max_pages=10")
	assert.Contains(t, progress.String(), "parsed_postings=1")
	assert.Contains(t, progress.String(), "crawler run persisted successfully")
}

func TestPersistCrawlResultsStoresPostingsAndCreatesRun(t *testing.T) {
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
	crawler := newTestCrawler(t, repo, noopCollect, nil, nil, nil)

	err := crawler.persistCrawlResults(context.Background(), postings, now.Add(-5*time.Minute), now)
	require.NoError(t, err)

	require.Len(t, repo.upsertedPostings, 1)
	require.NotNil(t, repo.createdCrawlRun)
	assert.Equal(t, postings, repo.upsertedPostings)
}

func timePtr(t time.Time) *time.Time {
	return &t
}

func newTestCrawler(
	t *testing.T,
	repo crawlRunRepository,
	collect collectFunc,
	now func() time.Time,
	out io.Writer,
	progressOut io.Writer,
) *Crawler {
	t.Helper()

	crawler, err := newCrawler(out, progressOut, repo, collect, now)
	require.NoError(t, err)

	return crawler
}

func noopCollect(_ context.Context, _ gamejob.CollectOptions) ([]model.JobPosting, error) {
	return nil, nil
}
