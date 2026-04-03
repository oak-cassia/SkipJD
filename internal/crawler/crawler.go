package crawler

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"google.golang.org/adk/agent"
	adkmodel "google.golang.org/adk/model"
	"google.golang.org/adk/model/gemini"
	"google.golang.org/adk/runner"
	"google.golang.org/adk/session"
	"google.golang.org/genai"

	"skipjd/internal/config"
	"skipjd/internal/repository"
)

const appName = "browser_agent"
const userID = "default_user"
const sessionID = "default_session"
const defaultTargetSite = "https://www.gamejob.co.kr/Recruit/Main"
const collectedPostingsKey = "collected_postings"

type AICrawler struct {
	out io.Writer

	crawlerRepository crawlRunRepository
	sessionService    session.Service
	rootAgent         agent.Agent
	mailer            Mailer
	now               func() time.Time
}

func NewAICrawler(ctx context.Context, configPath string, out io.Writer, crawlerRepository *repository.CrawlerRepository) (*AICrawler, error) {
	if crawlerRepository == nil {
		return nil, fmt.Errorf("crawler repository is required")
	}

	agentCfg, err := loadAgentConfig(configPath)
	if err != nil {
		return nil, fmt.Errorf("load agent config: %w", err)
	}

	modelInstance, err := newModel(ctx, agentCfg.ModelID)
	if err != nil {
		return nil, fmt.Errorf("create model: %w", err)
	}

	rootAgent, err := newBrowserAgent(agentCfg, modelInstance)
	if err != nil {
		return nil, fmt.Errorf("create agent: %w", err)
	}

	if out == nil {
		out = io.Discard
	}

	appCfg := config.Load()
	mailer := NewSMTPMailer(SMTPMailConfig{
		Host: appCfg.SMTPHost,
		Port: appCfg.SMTPPort,
		User: appCfg.SMTPUser,
		Pass: appCfg.SMTPPass,
		From: appCfg.MailFrom,
		To:   appCfg.MailTo,
	})

	return &AICrawler{
		out:               out,
		crawlerRepository: crawlerRepository,
		sessionService:    session.InMemoryService(),
		rootAgent:         rootAgent,
		mailer:            mailer,
		now:               time.Now,
	}, nil
}

func (c *AICrawler) Run(ctx context.Context) (err error) {
	defer c.closeBrowserSession()

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
	newPostings, err := c.findNewPostings(ctx, postings)
	if err != nil {
		return err
	}
	if err := c.persistCrawlResults(ctx, postings, startedAt, finishedAt); err != nil {
		return err
	}
	c.notifyNewPostings(ctx, finishedAt, newPostings)

	if _, err := fmt.Fprintln(c.out, outputText); err != nil {
		return fmt.Errorf("write crawl output: %w", err)
	}

	return nil
}

func (c *AICrawler) closeBrowserSession() {
	closeCtx, cancel := context.WithTimeout(context.Background(), agentBrowserDefaultTimeout)
	defer cancel()

	result := closeAgentBrowserSession(closeCtx)
	if result.ExitCode == 0 && result.Error == "" {
		return
	}

	out := c.out
	if out == nil {
		out = io.Discard
	}
	_, _ = fmt.Fprintf(
		out,
		"agent-browser close failed: exit_code=%d error=%s stderr=%s\n",
		result.ExitCode,
		strings.TrimSpace(result.Error),
		strings.TrimSpace(result.Stderr),
	)
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
