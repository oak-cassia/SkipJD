package crawler

import (
	"bytes"
	"context"
	"io"
	"sync/atomic"
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
	upsertedDuties   map[string][]int
	createdCrawlRun  *model.CrawlRun
	idsBySourceKey   map[string]uint
	upsertedBodies   map[uint]string
	bodyReadyForLLM  map[uint]bool
	replacedImages   map[uint][]string
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

func (r *stubCrawlRunRepository) UpsertJobPostings(ctx context.Context, postings []model.JobPosting, dutyCodesBySourceKey map[string][]int) error {
	_ = ctx
	r.upsertedPostings = append([]model.JobPosting(nil), postings...)
	r.upsertedDuties = cloneDutyCodesBySourceKey(dutyCodesBySourceKey)
	return nil
}

func (r *stubCrawlRunRepository) GetJobPostingIDsBySourceKeys(ctx context.Context, source string, sourceKeys []string) (map[string]uint, error) {
	_ = ctx
	_ = source
	out := make(map[string]uint, len(sourceKeys))
	for _, key := range sourceKeys {
		if id, ok := r.idsBySourceKey[key]; ok {
			out[key] = id
		}
	}
	return out, nil
}

func (r *stubCrawlRunRepository) UpsertJobPostingBodyHTML(ctx context.Context, jobPostingID uint, text string, readyForLLM bool) error {
	_ = ctx
	if r.upsertedBodies == nil {
		r.upsertedBodies = make(map[uint]string)
	}
	r.upsertedBodies[jobPostingID] = text
	if r.bodyReadyForLLM == nil {
		r.bodyReadyForLLM = make(map[uint]bool)
	}
	r.bodyReadyForLLM[jobPostingID] = readyForLLM
	return nil
}

func (r *stubCrawlRunRepository) ReplaceJobPostingImages(ctx context.Context, jobPostingID uint, urls []string) error {
	_ = ctx
	if r.replacedImages == nil {
		r.replacedImages = make(map[uint][]string)
	}
	r.replacedImages[jobPostingID] = append([]string(nil), urls...)
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

func TestToJobPostingsDedupesBySourceKey(t *testing.T) {
	seenAt := time.Date(2026, 3, 25, 8, 0, 0, 0, time.UTC)
	crawler := newTestCrawler(t, &stubCrawlRunRepository{}, noopCollect, nil, nil, nil)

	postings, dutyCodesBySourceKey := crawler.toJobPostings([]gamejob.ScrapedPosting{
		{
			SourceKey:          "https://www.gamejob.co.kr/Recruit/GI_Read/View?GI_No=275868",
			Title:              "Server Engineer",
			Company:            "㈜에피드게임즈",
			DutyCode:           1,
			ClosingDate:        "채용시",
			URL:                "https://www.gamejob.co.kr/Recruit/GI_Read/View?GI_No=275868",
			MinExperienceYears: 3,
		},
		{
			SourceKey:          "https://www.gamejob.co.kr/Recruit/GI_Read/View?GI_No=275868",
			Title:              "Duplicate",
			Company:            "에피드게임즈",
			DutyCode:           3,
			ClosingDate:        "채용시",
			URL:                "https://www.gamejob.co.kr/Recruit/GI_Read/View?GI_No=275868",
			MinExperienceYears: 1,
		},
		{
			SourceKey:          "https://www.gamejob.co.kr/Recruit/GI_Read/View?GI_No=275869",
			Title:              "Keep me",
			Company:            "크니브스튜디오",
			DutyCode:           16,
			ClosingDate:        "채용시",
			URL:                "https://www.gamejob.co.kr/Recruit/GI_Read/View?GI_No=275869",
			MinExperienceYears: 0,
		},
	}, seenAt)

	require.Len(t, postings, 2)
	assert.Equal(t, appName, postings[0].Source)
	assert.Equal(t, "Server Engineer", postings[0].Title)
	assert.Equal(t, "㈜에피드게임즈", postings[0].Company)
	assert.True(t, postings[0].FirstSeenAt.Equal(seenAt))
	assert.True(t, postings[0].LastSeenAt.Equal(seenAt))
	require.NotNil(t, postings[0].MinExperienceYears)
	assert.Equal(t, 3, *postings[0].MinExperienceYears)
	assert.Equal(t, "Keep me", postings[1].Title)
	assert.Equal(t, "크니브스튜디오", postings[1].Company)
	require.NotNil(t, postings[1].MinExperienceYears)
	assert.Equal(t, 0, *postings[1].MinExperienceYears)
	assert.Equal(t, map[string][]int{
		"https://www.gamejob.co.kr/Recruit/GI_Read/View?GI_No=275868": {1, 3},
		"https://www.gamejob.co.kr/Recruit/GI_Read/View?GI_No=275869": {16},
	}, dutyCodesBySourceKey)
}

func TestRunScrapesAndPersistsResults(t *testing.T) {
	scrapedPostings := []gamejob.ScrapedPosting{
		{
			SourceKey:          "https://www.gamejob.co.kr/Recruit/GI_Read/View?GI_No=275868",
			Title:              "Server Engineer",
			Company:            "㈜에피드게임즈",
			DutyCode:           1,
			ClosingDate:        "채용시",
			URL:                "https://www.gamejob.co.kr/Recruit/GI_Read/View?GI_No=275868",
			MinExperienceYears: 3,
			ObservedDate:       time.Date(2026, 3, 25, 0, 0, 0, 0, seoulLocation),
		},
		{
			SourceKey:          "https://www.gamejob.co.kr/Recruit/GI_Read/View?GI_No=275868",
			Title:              "Duplicate",
			Company:            "에피드게임즈",
			DutyCode:           3,
			ClosingDate:        "채용시",
			URL:                "https://www.gamejob.co.kr/Recruit/GI_Read/View?GI_No=275868",
			MinExperienceYears: 1,
			ObservedDate:       time.Date(2026, 3, 25, 0, 0, 0, 0, seoulLocation),
		},
		{
			SourceKey:          "https://www.gamejob.co.kr/Recruit/GI_Read/View?GI_No=275869",
			Title:              "Keep me",
			Company:            "크니브스튜디오",
			DutyCode:           16,
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
	assert.JSONEq(t, `{"postings":[{"title":"Server Engineer","company":"㈜에피드게임즈","duty_codes":[1,3],"url":"https://www.gamejob.co.kr/Recruit/GI_Read/View?GI_No=275868","closing_date":"채용시","min_experience_years":3},{"title":"Keep me","company":"크니브스튜디오","duty_codes":[16],"url":"https://www.gamejob.co.kr/Recruit/GI_Read/View?GI_No=275869","closing_date":"채용시","min_experience_years":0}]}`, out.String())
	require.Len(t, repo.upsertedPostings, 2)
	assert.Equal(t, appName, repo.upsertedPostings[0].Source)
	assert.Equal(t, "https://www.gamejob.co.kr/Recruit/GI_Read/View?GI_No=275868", repo.upsertedPostings[0].SourceKey)
	assert.Equal(t, "Server Engineer", repo.upsertedPostings[0].Title)
	assert.Equal(t, "㈜에피드게임즈", repo.upsertedPostings[0].Company)
	assert.True(t, repo.upsertedPostings[0].FirstSeenAt.Equal(repo.upsertedPostings[0].LastSeenAt))
	require.NotNil(t, repo.upsertedPostings[0].MinExperienceYears)
	assert.Equal(t, 3, *repo.upsertedPostings[0].MinExperienceYears)
	assert.Equal(t, "https://www.gamejob.co.kr/Recruit/GI_Read/View?GI_No=275869", repo.upsertedPostings[1].SourceKey)
	assert.Equal(t, "Keep me", repo.upsertedPostings[1].Title)
	assert.Equal(t, "크니브스튜디오", repo.upsertedPostings[1].Company)
	require.NotNil(t, repo.upsertedPostings[1].MinExperienceYears)
	assert.Equal(t, 0, *repo.upsertedPostings[1].MinExperienceYears)
	assert.Equal(t, map[string][]int{
		"https://www.gamejob.co.kr/Recruit/GI_Read/View?GI_No=275868": {1, 3},
		"https://www.gamejob.co.kr/Recruit/GI_Read/View?GI_No=275869": {16},
	}, repo.upsertedDuties)
	require.NotNil(t, repo.createdCrawlRun)
	assert.Equal(t, appName, repo.createdCrawlRun.Source)
	assert.Contains(t, progress.String(), "collect_options last_updated=2026-03-21 today_date=2026-03-25 max_pages=10")
	assert.Contains(t, progress.String(), "parsed_postings=2")
	assert.Contains(t, progress.String(), "crawler run persisted successfully")
}

func TestRunEnrichesAndPersistsBodiesAndImages(t *testing.T) {
	scrapedPostings := []gamejob.ScrapedPosting{
		{
			SourceKey:    "https://www.gamejob.co.kr/Recruit/GI_Read/View?GI_No=275868",
			Title:        "Server Engineer",
			Company:      "에피드게임즈",
			DutyCode:     1,
			ClosingDate:  "채용시",
			URL:          "https://www.gamejob.co.kr/Recruit/GI_Read/View?GI_No=275868",
			ObservedDate: time.Date(2026, 3, 25, 0, 0, 0, 0, seoulLocation),
		},
		{
			SourceKey:    "https://www.gamejob.co.kr/Recruit/GI_Read/View?GI_No=275869",
			Title:        "Image Posting",
			Company:      "크니브스튜디오",
			DutyCode:     16,
			ClosingDate:  "채용시",
			URL:          "https://www.gamejob.co.kr/Recruit/GI_Read/View?GI_No=275869",
			ObservedDate: time.Date(2026, 3, 25, 0, 0, 0, 0, seoulLocation),
		},
	}
	repo := &stubCrawlRunRepository{
		latestFinishedAt: timePtr(time.Date(2026, 3, 20, 23, 30, 0, 0, time.UTC)),
		idsBySourceKey: map[string]uint{
			"https://www.gamejob.co.kr/Recruit/GI_Read/View?GI_No=275868": 101,
			"https://www.gamejob.co.kr/Recruit/GI_Read/View?GI_No=275869": 102,
		},
	}
	collector := &stubCollector{postings: scrapedPostings}
	detailByURL := map[string]gamejob.DetailContent{
		"https://www.gamejob.co.kr/Recruit/GI_Read/View?GI_No=275868": {
			TextContent: "본문 텍스트입니다.",
			ImageURLs:   []string{"https://imgs.gamejob.co.kr/ext/x.png"},
		},
		"https://www.gamejob.co.kr/Recruit/GI_Read/View?GI_No=275869": {
			ImageURLs: []string{"https://imgs.gamejob.co.kr/ext/y.png"},
		},
	}

	crawler, err := newCrawler(repo, collector.Scrape,
		WithNowFunc(func() time.Time {
			return time.Date(2026, 3, 25, 8, 0, 0, 0, time.UTC)
		}),
		WithDetailCollector(func(_ context.Context, postingURL string) (gamejob.DetailContent, error) {
			return detailByURL[postingURL], nil
		}),
	)
	require.NoError(t, err)

	require.NoError(t, crawler.Run(context.Background()))

	assert.Equal(t, "본문 텍스트입니다.", repo.upsertedBodies[101])
	_, hasBody102 := repo.upsertedBodies[102]
	assert.False(t, hasBody102, "image-only posting should not get an html body row")
	assert.False(t, repo.bodyReadyForLLM[101], "body with pending images should not be ready for LLM")
	assert.Equal(t, []string{"https://imgs.gamejob.co.kr/ext/x.png"}, repo.replacedImages[101])
	assert.Equal(t, []string{"https://imgs.gamejob.co.kr/ext/y.png"}, repo.replacedImages[102])
}

func TestRunMarksReadyForLLMWhenNoImages(t *testing.T) {
	scrapedPostings := []gamejob.ScrapedPosting{
		{
			SourceKey:    "https://www.gamejob.co.kr/Recruit/GI_Read/View?GI_No=275870",
			Title:        "Text Only Posting",
			Company:      "텍스트회사",
			DutyCode:     1,
			ClosingDate:  "채용시",
			URL:          "https://www.gamejob.co.kr/Recruit/GI_Read/View?GI_No=275870",
			ObservedDate: time.Date(2026, 3, 25, 0, 0, 0, 0, seoulLocation),
		},
	}
	repo := &stubCrawlRunRepository{
		latestFinishedAt: timePtr(time.Date(2026, 3, 20, 23, 30, 0, 0, time.UTC)),
		idsBySourceKey: map[string]uint{
			"https://www.gamejob.co.kr/Recruit/GI_Read/View?GI_No=275870": 201,
		},
	}
	collector := &stubCollector{postings: scrapedPostings}
	detailByURL := map[string]gamejob.DetailContent{
		"https://www.gamejob.co.kr/Recruit/GI_Read/View?GI_No=275870": {
			TextContent: "이 공고는 이미지가 없습니다.",
		},
	}

	crawler, err := newCrawler(repo, collector.Scrape,
		WithNowFunc(func() time.Time {
			return time.Date(2026, 3, 25, 8, 0, 0, 0, time.UTC)
		}),
		WithDetailCollector(func(_ context.Context, postingURL string) (gamejob.DetailContent, error) {
			return detailByURL[postingURL], nil
		}),
	)
	require.NoError(t, err)

	require.NoError(t, crawler.Run(context.Background()))

	assert.Equal(t, "이 공고는 이미지가 없습니다.", repo.upsertedBodies[201])
	assert.True(t, repo.bodyReadyForLLM[201], "body without images should be ready for LLM")
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

	err := crawler.persistCrawlResults(context.Background(), postings, map[string][]int{
		"jobs/example/1": {3},
	}, nil, now.Add(-5*time.Minute), now)
	require.NoError(t, err)

	require.Len(t, repo.upsertedPostings, 1)
	assert.Equal(t, map[string][]int{"jobs/example/1": {3}}, repo.upsertedDuties)
	require.NotNil(t, repo.createdCrawlRun)
	assert.Equal(t, postings, repo.upsertedPostings)
}

func timePtr(t time.Time) *time.Time {
	return &t
}

func TestEnrichWithDetailHonorsWorkerLimit(t *testing.T) {
	const workers = 2
	const total = 10

	var inFlight, peak atomic.Int32
	collectDetail := func(ctx context.Context, _ string) (gamejob.DetailContent, error) {
		current := inFlight.Add(1)
		for {
			p := peak.Load()
			if current <= p || peak.CompareAndSwap(p, current) {
				break
			}
		}
		// Hold the slot long enough that the limit is observable.
		time.Sleep(20 * time.Millisecond)
		inFlight.Add(-1)
		return gamejob.DetailContent{TextContent: "ok"}, nil
	}

	crawler, err := newCrawler(&stubCrawlRunRepository{}, noopCollect,
		WithDetailCollector(collectDetail),
		WithDetailWorkers(workers),
	)
	require.NoError(t, err)

	postings := make([]model.JobPosting, total)
	for i := range postings {
		postings[i] = model.JobPosting{SourceKey: "k" + string(rune('0'+i)), URL: "http://example.test"}
	}

	out := crawler.enrichWithDetail(context.Background(), postings)
	assert.Len(t, out, total, "all postings should produce content")
	assert.LessOrEqual(t, peak.Load(), int32(workers),
		"at most %d concurrent collectDetail calls, got peak=%d", workers, peak.Load())
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

	var opts []Option
	if out != nil {
		opts = append(opts, WithOutput(out))
	}
	if progressOut != nil {
		opts = append(opts, WithProgressOutput(progressOut))
	}
	if now != nil {
		opts = append(opts, WithNowFunc(now))
	}

	crawler, err := newCrawler(repo, collect, opts...)
	require.NoError(t, err)

	return crawler
}

func noopCollect(_ context.Context, _ gamejob.ScrapeOptions) ([]gamejob.ScrapedPosting, error) {
	return nil, nil
}

func cloneDutyCodesBySourceKey(values map[string][]int) map[string][]int {
	if len(values) == 0 {
		return nil
	}

	cloned := make(map[string][]int, len(values))
	for sourceKey, dutyCodes := range values {
		copied := make([]int, len(dutyCodes))
		copy(copied, dutyCodes)
		cloned[sourceKey] = copied
	}

	return cloned
}
