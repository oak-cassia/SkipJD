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
	postings []gamejob.ScrapedPosting
	lastOpts gamejob.ScrapeOptions
	calls    int
}

func (c *stubCollector) Scrape(_ context.Context, opts gamejob.ScrapeOptions) ([]gamejob.ScrapedPosting, error) {
	c.calls++
	c.lastOpts = opts
	return append([]gamejob.ScrapedPosting(nil), c.postings...), nil
}

func TestCanonicalCompanyNameStripsCorporateMarkers(t *testing.T) {
	assert.Equal(t, "에피드게임즈", canonicalCompanyName("㈜에피드게임즈"))
	assert.Equal(t, "드림모션", canonicalCompanyName("(주) 드림모션"))
	assert.Equal(t, "웹젠", canonicalCompanyName("주식회사 웹젠"))
}

func TestToJobPostingsFiltersPreferredCompaniesAndDedupes(t *testing.T) {
	seenAt := time.Date(2026, 3, 25, 8, 0, 0, 0, time.UTC)
	crawler := newTestCrawler(t, &stubCrawlRunRepository{}, noopCollect, nil, nil, nil)

	postings := crawler.toJobPostings([]gamejob.ScrapedPosting{
		{
			SourceKey:          "https://www.gamejob.co.kr/Recruit/GI_Read/View?GI_No=275868",
			Title:              "Server Engineer",
			Company:            "㈜에피드게임즈",
			ClosingDate:        "채용시",
			URL:                "https://www.gamejob.co.kr/Recruit/GI_Read/View?GI_No=275868",
			MinExperienceYears: 3,
		},
		{
			SourceKey:          "https://www.gamejob.co.kr/Recruit/GI_Read/View?GI_No=275868",
			Title:              "Duplicate",
			Company:            "에피드게임즈",
			ClosingDate:        "채용시",
			URL:                "https://www.gamejob.co.kr/Recruit/GI_Read/View?GI_No=275868",
			MinExperienceYears: 1,
		},
		{
			SourceKey:          "https://www.gamejob.co.kr/Recruit/GI_Read/View?GI_No=275869",
			Title:              "Skip me",
			Company:            "크니브스튜디오",
			ClosingDate:        "채용시",
			URL:                "https://www.gamejob.co.kr/Recruit/GI_Read/View?GI_No=275869",
			MinExperienceYears: 0,
		},
	}, []string{"에피드게임즈"}, seenAt)

	require.Len(t, postings, 1)
	assert.Equal(t, appName, postings[0].Source)
	assert.Equal(t, "Server Engineer", postings[0].Title)
	assert.Equal(t, "㈜에피드게임즈", postings[0].Company)
	assert.True(t, postings[0].FirstSeenAt.Equal(seenAt))
	assert.True(t, postings[0].LastSeenAt.Equal(seenAt))
	require.NotNil(t, postings[0].MinExperienceYears)
	assert.Equal(t, 3, *postings[0].MinExperienceYears)
}

func TestRunScrapesFiltersAndPersistsResults(t *testing.T) {
	scrapedPostings := []gamejob.ScrapedPosting{
		{
			SourceKey:          "https://www.gamejob.co.kr/Recruit/GI_Read/View?GI_No=275868",
			Title:              "Server Engineer",
			Company:            "㈜에피드게임즈",
			ClosingDate:        "채용시",
			URL:                "https://www.gamejob.co.kr/Recruit/GI_Read/View?GI_No=275868",
			MinExperienceYears: 3,
			ObservedDate:       time.Date(2026, 3, 25, 0, 0, 0, 0, seoulLocation),
		},
		{
			SourceKey:          "https://www.gamejob.co.kr/Recruit/GI_Read/View?GI_No=275868",
			Title:              "Duplicate",
			Company:            "에피드게임즈",
			ClosingDate:        "채용시",
			URL:                "https://www.gamejob.co.kr/Recruit/GI_Read/View?GI_No=275868",
			MinExperienceYears: 1,
			ObservedDate:       time.Date(2026, 3, 25, 0, 0, 0, 0, seoulLocation),
		},
		{
			SourceKey:          "https://www.gamejob.co.kr/Recruit/GI_Read/View?GI_No=275869",
			Title:              "Skip me",
			Company:            "크니브스튜디오",
			ClosingDate:        "채용시",
			URL:                "https://www.gamejob.co.kr/Recruit/GI_Read/View?GI_No=275869",
			MinExperienceYears: 0,
			ObservedDate:       time.Date(2026, 3, 25, 0, 0, 0, 0, seoulLocation),
		},
	}
	repo := &stubCrawlRunRepository{
		latestFinishedAt: timePtr(time.Date(2026, 3, 20, 23, 30, 0, 0, time.UTC)),
	}
	collector := &stubCollector{postings: scrapedPostings}
	var out bytes.Buffer
	var progress bytes.Buffer

	crawler := newTestCrawler(t, repo, collector.Scrape, func() time.Time {
		return time.Date(2026, 3, 25, 8, 0, 0, 0, time.UTC)
	}, &out, &progress)

	err := crawler.Run(context.Background())
	require.NoError(t, err)

	assert.Equal(t, 1, collector.calls)
	assert.Equal(t, "2026-03-25", collector.lastOpts.TodayDate.In(seoulLocation).Format(dateOnlyFormat))
	assert.Equal(t, gamejob.DefaultMaxPages, collector.lastOpts.MaxPages)
	require.NotNil(t, collector.lastOpts.Stop)
	assert.True(t, collector.lastOpts.Stop(gamejob.ScrapedPosting{
		ObservedDate: time.Date(2026, 3, 20, 0, 0, 0, 0, seoulLocation),
	}))
	assert.False(t, collector.lastOpts.Stop(gamejob.ScrapedPosting{
		ObservedDate: time.Date(2026, 3, 21, 0, 0, 0, 0, seoulLocation),
	}))
	assert.JSONEq(t, `{"postings":[{"title":"Server Engineer","company":"㈜에피드게임즈","url":"https://www.gamejob.co.kr/Recruit/GI_Read/View?GI_No=275868","closing_date":"채용시","min_experience_years":3}]}`, out.String())
	require.Len(t, repo.upsertedPostings, 1)
	assert.Equal(t, appName, repo.upsertedPostings[0].Source)
	assert.Equal(t, "https://www.gamejob.co.kr/Recruit/GI_Read/View?GI_No=275868", repo.upsertedPostings[0].SourceKey)
	assert.Equal(t, "Server Engineer", repo.upsertedPostings[0].Title)
	assert.Equal(t, "㈜에피드게임즈", repo.upsertedPostings[0].Company)
	assert.True(t, repo.upsertedPostings[0].FirstSeenAt.Equal(repo.upsertedPostings[0].LastSeenAt))
	require.NotNil(t, repo.upsertedPostings[0].MinExperienceYears)
	assert.Equal(t, 3, *repo.upsertedPostings[0].MinExperienceYears)
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

func noopCollect(_ context.Context, _ gamejob.ScrapeOptions) ([]gamejob.ScrapedPosting, error) {
	return nil, nil
}
