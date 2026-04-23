package crawler

import (
	"context"
	"fmt"
	"io"
	"os"
	"time"

	"skipjd/internal/gamejob"
	"skipjd/internal/repository"
)

const appName = gamejob.SourceName

type collectFunc func(ctx context.Context, opts gamejob.ScrapeOptions) ([]gamejob.ScrapedPosting, error)

type Crawler struct {
	out         io.Writer
	progressOut io.Writer

	crawlerRepository crawlRunRepository
	collect           collectFunc
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

	return newCrawler(crawlerRepository, scraper.Scrape, WithOutput(out))
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
	finishedAt := c.now().Local()
	outputText, err := Encode(postings, dutyCodesBySourceKey)
	if err != nil {
		return fmt.Errorf("encode collected postings: %w", err)
	}
	if _, err := fmt.Fprintf(c.progressWriter(), "parsed_postings=%d\n", len(postings)); err != nil {
		return fmt.Errorf("write progress output: %w", err)
	}
	if err := c.persistCrawlResults(ctx, postings, dutyCodesBySourceKey, startedAt, finishedAt); err != nil {
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
	crawler, err := newCrawler(crawlerRepository, scraper.Scrape, WithOutput(os.Stdout), WithProgressOutput(os.Stderr))
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
