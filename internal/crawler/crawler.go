package crawler

import (
	"context"
	"fmt"
	"io"
	"os"
	"sync"
	"time"

	"skipjd/internal/gamejob"
	"skipjd/internal/model"
	"skipjd/internal/repository"
)

const (
	appName           = gamejob.SourceName
	detailWorkerCount = 5
)

type collectFunc func(ctx context.Context, opts gamejob.ScrapeOptions) ([]gamejob.ScrapedPosting, error)
type detailCollectFunc func(ctx context.Context, postingURL string) (gamejob.DetailContent, error)

type Crawler struct {
	out         io.Writer
	progressOut io.Writer

	crawlerRepository crawlRunRepository
	collect           collectFunc
	collectDetail     detailCollectFunc
	now               func() time.Time
}

type Option func(*Crawler)

func WithOutput(out io.Writer) Option {
	return func(c *Crawler) {
		if out != nil {
			c.out = out
		}
	}
}

func WithProgressOutput(progressOut io.Writer) Option {
	return func(c *Crawler) {
		if progressOut != nil {
			c.progressOut = progressOut
		}
	}
}

func WithNowFunc(now func() time.Time) Option {
	return func(c *Crawler) {
		if now != nil {
			c.now = now
		}
	}
}

func WithDetailCollector(detail detailCollectFunc) Option {
	return func(c *Crawler) {
		if detail != nil {
			c.collectDetail = detail
		}
	}
}

func newCrawler(
	crawlerRepository crawlRunRepository,
	collect collectFunc,
	opts ...Option,
) (*Crawler, error) {
	if crawlerRepository == nil {
		return nil, fmt.Errorf("crawler repository is required")
	}
	if collect == nil {
		return nil, fmt.Errorf("crawler collector is required")
	}

	c := &Crawler{
		out:               io.Discard,
		progressOut:       io.Discard,
		crawlerRepository: crawlerRepository,
		collect:           collect,
		now:               time.Now,
	}

	for _, opt := range opts {
		opt(c)
	}

	return c, nil
}

func NewCrawler(out io.Writer, crawlerRepository *repository.CrawlerRepository) (*Crawler, error) {
	scraper := gamejob.NewClientScraper(nil)
	detailScraper := gamejob.NewDetailScraper(nil)

	return newCrawler(crawlerRepository, scraper.Scrape, WithOutput(out), WithDetailCollector(detailScraper.Scrape))
}

func (c *Crawler) Run(ctx context.Context) error {
	startedAt := c.now().Local()

	opts, err := c.buildCollectOptions(ctx)
	if err != nil {
		return err
	}
	if _, err := fmt.Fprintf(
		c.progressWriter(),
		"collect_options last_updated=%s today_date=%s max_pages=%d\n",
		opts.LastUpdated.In(seoulLocation).Format(dateOnlyFormat),
		opts.TodayDate.In(seoulLocation).Format(dateOnlyFormat),
		opts.MaxPages,
	); err != nil {
		return fmt.Errorf("write collect options output: %w", err)
	}

	scrapedPostings, err := c.collect(ctx, gamejob.ScrapeOptions{
		TodayDate: opts.TodayDate,
		MaxPages:  opts.MaxPages,
		Stop: func(scraped gamejob.ScrapedPosting) bool {
			return scraped.ObservedDate.Before(opts.LastUpdated)
		},
	})
	if err != nil {
		return fmt.Errorf("scrape postings: %w", err)
	}

	postings, dutyCodesBySourceKey := c.toJobPostings(scrapedPostings, startedAt)
	detailBySourceKey := c.enrichWithDetail(ctx, postings)
	finishedAt := c.now().Local()
	outputText, err := Encode(postings, dutyCodesBySourceKey)
	if err != nil {
		return fmt.Errorf("encode collected postings: %w", err)
	}
	if _, err := fmt.Fprintf(c.progressWriter(), "parsed_postings=%d enriched_postings=%d\n", len(postings), len(detailBySourceKey)); err != nil {
		return fmt.Errorf("write progress output: %w", err)
	}
	if err := c.persistCrawlResults(ctx, postings, dutyCodesBySourceKey, detailBySourceKey, startedAt, finishedAt); err != nil {
		return err
	}
	if _, err := fmt.Fprintln(c.progressWriter(), "crawler run persisted successfully"); err != nil {
		return fmt.Errorf("write progress output: %w", err)
	}

	if _, err := io.WriteString(c.out, outputText); err != nil {
		return fmt.Errorf("write crawl output: %w", err)
	}

	return nil
}

func Run(ctx context.Context, crawlerRepository *repository.CrawlerRepository) error {
	scraper := gamejob.NewClientScraper(nil)
	detailScraper := gamejob.NewDetailScraper(nil)
	crawler, err := newCrawler(crawlerRepository, scraper.Scrape,
		WithOutput(os.Stdout),
		WithProgressOutput(os.Stderr),
		WithDetailCollector(detailScraper.Scrape),
	)
	if err != nil {
		return err
	}

	return crawler.Run(ctx)
}

func (c *Crawler) progressWriter() io.Writer {
	if c.progressOut == nil {
		return io.Discard
	}

	return c.progressOut
}

// enrichWithDetail fetches body+image content for each posting using a worker
// pool. Failures are logged to progress writer but don't abort the crawl —
// metadata is still persisted, body just stays empty.
func (c *Crawler) enrichWithDetail(ctx context.Context, postings []model.JobPosting) map[string]gamejob.DetailContent {
	if c.collectDetail == nil || len(postings) == 0 {
		return nil
	}

	type result struct {
		sourceKey string
		content   gamejob.DetailContent
		err       error
	}

	sem := make(chan struct{}, detailWorkerCount)
	resultsChan := make(chan result, len(postings))
	var wg sync.WaitGroup

	for _, posting := range postings {
		wg.Add(1)
		go func() {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			content, err := c.collectDetail(ctx, posting.URL)
			resultsChan <- result{sourceKey: posting.SourceKey, content: content, err: err}
		}()
	}

	go func() {
		wg.Wait()
		close(resultsChan)
	}()

	out := make(map[string]gamejob.DetailContent, len(postings))
	failed := 0
	for r := range resultsChan {
		if r.err != nil {
			failed++
			_, _ = fmt.Fprintf(c.progressWriter(), "detail fetch failed source_key=%s err=%v\n", r.sourceKey, r.err)
			continue
		}
		if r.content.TextContent == "" && len(r.content.ImageURLs) == 0 {
			continue
		}
		out[r.sourceKey] = r.content
	}
	if failed > 0 {
		_, _ = fmt.Fprintf(c.progressWriter(), "detail fetch failures=%d\n", failed)
	}
	return out
}
