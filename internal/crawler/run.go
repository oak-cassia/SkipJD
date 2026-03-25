package crawler

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"os"
	"strings"
	"time"

	"google.golang.org/adk/agent"
	adkmodel "google.golang.org/adk/model"
	"google.golang.org/adk/model/gemini"
	"google.golang.org/adk/runner"
	"google.golang.org/adk/session"
	"google.golang.org/genai"

	"skipjd/internal/model"
	"skipjd/internal/repository"
)

const appName = "browser_agent"
const userID = "default_user"
const sessionID = "default_session"
const defaultTargetSite = "target_site"
const collectedPostingsKey = "collected_postings"

type AICrawler struct {
	out io.Writer

	crawlerRepository crawlRunRepository
	sessionService    session.Service
	rootAgent         agent.Agent
	now               func() time.Time
}

func NewAICrawler(ctx context.Context, configPath string, out io.Writer, crawlerRepository *repository.CrawlerRepository) (*AICrawler, error) {
	if crawlerRepository == nil {
		return nil, fmt.Errorf("crawler repository is required")
	}

	cfg, err := loadAgentConfig(configPath)
	if err != nil {
		return nil, fmt.Errorf("load agent config: %w", err)
	}

	modelInstance, err := newModel(ctx, cfg.ModelID)
	if err != nil {
		return nil, fmt.Errorf("create model: %w", err)
	}

	rootAgent, err := newBrowserAgent(cfg, modelInstance)
	if err != nil {
		return nil, fmt.Errorf("create agent: %w", err)
	}

	if out == nil {
		out = io.Discard
	}

	return &AICrawler{
		out:               out,
		crawlerRepository: crawlerRepository,
		sessionService:    session.InMemoryService(),
		rootAgent:         rootAgent,
		now:               time.Now,
	}, nil
}

func (c *AICrawler) Run(ctx context.Context) (err error) {
	startedAt := c.now().Local()

	state, err := c.buildSessionState(ctx)
	if err != nil {
		return err
	}

	resp, err := c.sessionService.Create(ctx, &session.CreateRequest{
		AppName:   appName,
		UserID:    userID,
		SessionID: sessionID,
		State:     state,
	})
	if err != nil {
		return fmt.Errorf("create session: %w", err)
	}

	r, err := runner.New(runner.Config{
		AppName:        appName,
		Agent:          c.rootAgent,
		SessionService: c.sessionService,
	})
	if err != nil {
		return fmt.Errorf("create runner: %w", err)
	}

	currentSession := resp.Session
	for event, err := range r.Run(
		ctx,
		userID,
		currentSession.ID(),
		genai.NewContentFromText("run", "user"),
		agent.RunConfig{},
	) {
		if err != nil {
			return fmt.Errorf("agent run failed: %w", err)
		}

		if _, err := fmt.Fprintf(c.out, "Event: %s partial=%v author=%s\n", event.ID, event.Partial, event.Author); err != nil {
			return fmt.Errorf("write event output: %w", err)
		}

		if event.UsageMetadata != nil {
			u := event.UsageMetadata
			if _, err := fmt.Fprintf(
				c.out,
				"tokens prompt=%d candidates=%d thoughts=%d tool_use=%d cached=%d total=%d\n",
				u.PromptTokenCount,
				u.CandidatesTokenCount,
				u.ThoughtsTokenCount,
				u.ToolUsePromptTokenCount,
				u.CachedContentTokenCount,
				u.TotalTokenCount,
			); err != nil {
				return fmt.Errorf("write token output: %w", err)
			}
		}
	}

	getResp, err := c.sessionService.Get(ctx, &session.GetRequest{
		AppName:   appName,
		UserID:    userID,
		SessionID: currentSession.ID(),
	})
	if err != nil {
		return fmt.Errorf("load session: %w", err)
	}

	output, err := getResp.Session.State().Get(collectedPostingsKey)
	if err != nil {
		return fmt.Errorf("load %s from session state: %w", collectedPostingsKey, err)
	}

	outputText, ok := output.(string)
	if !ok {
		return fmt.Errorf("unexpected %s type: %T", collectedPostingsKey, output)
	}

	finishedAt := c.now().Local()
	postings, err := parseCollectedPostings(outputText, finishedAt)
	if err != nil {
		return fmt.Errorf("parse collected postings: %w", err)
	}
	if err := c.crawlerRepository.UpsertJobPostings(ctx, postings); err != nil {
		return fmt.Errorf("upsert job postings: %w", err)
	}

	if err := c.crawlerRepository.CreateCrawlRun(ctx, &model.CrawlRun{
		Source:     appName,
		StartedAt:  startedAt,
		FinishedAt: finishedAt,
	}); err != nil {
		return fmt.Errorf("create crawl run: %w", err)
	}

	if _, err := fmt.Fprintln(c.out, outputText); err != nil {
		return fmt.Errorf("write crawl output: %w", err)
	}

	return nil
}

func Run(ctx context.Context, configPath string, crawlerRepository *repository.CrawlerRepository) error {
	crawler, err := NewAICrawler(ctx, configPath, os.Stdout, crawlerRepository)
	if err != nil {
		return err
	}

	return crawler.Run(ctx)
}

func newModel(ctx context.Context, modelID string) (adkmodel.LLM, error) {
	return gemini.NewModel(ctx, modelID, &genai.ClientConfig{
		APIKey: os.Getenv("GOOGLE_API_KEY"),
	})
}

type crawlRunRepository interface {
	CreateCrawlRun(ctx context.Context, crawlRun *model.CrawlRun) error
	GetLatestFinishedAtBySource(ctx context.Context, source string) (*time.Time, error)
	UpsertJobPostings(ctx context.Context, postings []model.JobPosting) error
}

func (c *AICrawler) buildSessionState(ctx context.Context) (map[string]any, error) {
	lastUpdated, err := c.resolveLastUpdated(ctx)
	if err != nil {
		return nil, err
	}

	return map[string]any{
		"target_site": defaultTargetSite,
		"preferred_companies": []string{
			"크래프톤",
		},
		"preferred_positions": []string{
			"게임 서버", "AX",
		},
		"last_updated": lastUpdated.Format(time.RFC3339),
	}, nil
}

func (c *AICrawler) resolveLastUpdated(ctx context.Context) (time.Time, error) {
	latestFinishedAt, err := c.crawlerRepository.GetLatestFinishedAtBySource(ctx, appName)
	if err != nil {
		return time.Time{}, fmt.Errorf("get latest finished_at by source: %w", err)
	}
	if latestFinishedAt != nil {
		return *latestFinishedAt, nil
	}

	return c.now().AddDate(0, 0, -14), nil
}

type collectedPostingsOutput struct {
	Postings []collectedPosting `json:"postings"`
}

type collectedPosting struct {
	Title              string `json:"title"`
	Company            string `json:"company"`
	URL                string `json:"url"`
	ClosingDate        string `json:"closing_date"`
	MinExperienceYears *int   `json:"min_experience_years"`
}

func parseCollectedPostings(outputText string, seenAt time.Time) ([]model.JobPosting, error) {
	var output collectedPostingsOutput
	if err := json.Unmarshal([]byte(outputText), &output); err != nil {
		return nil, err
	}

	postings := make([]model.JobPosting, 0, len(output.Postings))
	for i, posting := range output.Postings {
		title := strings.TrimSpace(posting.Title)
		company := strings.TrimSpace(posting.Company)
		postingURL := strings.TrimSpace(posting.URL)
		closingDate := strings.TrimSpace(posting.ClosingDate)

		if title == "" {
			return nil, fmt.Errorf("posting %d: title is required", i)
		}
		if company == "" {
			return nil, fmt.Errorf("posting %d: company is required", i)
		}
		if postingURL == "" {
			return nil, fmt.Errorf("posting %d: url is required", i)
		}
		if closingDate == "" {
			return nil, fmt.Errorf("posting %d: closing_date is required", i)
		}

		sourceKey, err := buildSourceKey(postingURL)
		if err != nil {
			return nil, fmt.Errorf("posting %d: %w", i, err)
		}

		var minExperienceYears *int
		if posting.MinExperienceYears != nil {
			if *posting.MinExperienceYears < 0 {
				return nil, fmt.Errorf("posting %d: min_experience_years must be non-negative", i)
			}
			minExperienceYears = new(*posting.MinExperienceYears)
		}

		postings = append(postings, model.JobPosting{
			Source:             appName,
			SourceKey:          sourceKey,
			Title:              title,
			Company:            company,
			URL:                postingURL,
			ClosingDate:        closingDate,
			MinExperienceYears: minExperienceYears,
			FirstSeenAt:        seenAt,
			LastSeenAt:         seenAt,
		})
	}

	return postings, nil
}

func buildSourceKey(rawURL string) (string, error) {
	trimmed := strings.TrimSpace(rawURL)
	if trimmed == "" {
		return "", fmt.Errorf("source key requires a non-empty url")
	}

	parsedURL, err := url.Parse(trimmed)
	if err != nil {
		return trimmed, nil
	}

	parsedURL.Fragment = ""
	normalized := parsedURL.String()
	if normalized == "" {
		return trimmed, nil
	}

	return normalized, nil
}
