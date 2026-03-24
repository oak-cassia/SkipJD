package crawler

import (
	"context"
	"fmt"
	"io"
	"os"

	"google.golang.org/adk/agent"
	"google.golang.org/adk/model"
	"google.golang.org/adk/model/gemini"
	"google.golang.org/adk/runner"
	"google.golang.org/adk/session"
	"google.golang.org/genai"

	"skipjd/internal/repository"
)

const appName = "browser_agent"
const userID = "default_user"
const sessionID = "default_session"
const defaultTargetSite = "target_site"
const collectedPostingsKey = "collected_postings"

type AICrawler struct {
	out io.Writer

	crawlerRepository repository.CrawlerRepository
	sessionService    session.Service
	rootAgent         agent.Agent
}

func NewAICrawler(ctx context.Context, configPath string, out io.Writer, crawlerRepository repository.CrawlerRepository) (*AICrawler, error) {
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
	}, nil
}

func (c *AICrawler) Run(ctx context.Context) error {
	resp, err := c.sessionService.Create(ctx, &session.CreateRequest{
		AppName:   appName,
		UserID:    userID,
		SessionID: sessionID,
		State: map[string]any{
			"target_site": defaultTargetSite,
			"preferred_companies": []string{
				"데브시스터즈",
			},
			"preferred_positions": []string{
				"게임 서버",
			},
		},
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

	if _, err := fmt.Fprintln(c.out, outputText); err != nil {
		return fmt.Errorf("write crawl output: %w", err)
	}

	return nil
}

func Run(ctx context.Context, configPath string) error {
	crawler, err := NewAICrawler(ctx, configPath, os.Stdout, repository.CrawlerRepository{})
	if err != nil {
		return err
	}

	return crawler.Run(ctx)
}

func newModel(ctx context.Context, modelID string) (model.LLM, error) {
	return gemini.NewModel(ctx, modelID, &genai.ClientConfig{
		APIKey: os.Getenv("GOOGLE_API_KEY"),
	})
}
