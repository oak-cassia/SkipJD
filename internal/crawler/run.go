package crawler

import (
	"context"
	"fmt"

	"google.golang.org/adk/agent"
	"google.golang.org/adk/runner"
	"google.golang.org/adk/session"
	"google.golang.org/genai"
)

const appName = "browser_agent"
const userID = "default_user"
const sessionID = "default_session"
const defaultTargetSite = "target_site"
const collectedPostingsKey = "collected_postings"

func Run(ctx context.Context, configPath string) error {
	rootAgent, err := newBrowserAgent(ctx, configPath)
	if err != nil {
		return fmt.Errorf("create agent: %w", err)
	}

	sessionService := session.InMemoryService()
	resp, err := sessionService.Create(ctx, &session.CreateRequest{
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
		Agent:          rootAgent,
		SessionService: sessionService,
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

		fmt.Printf("Event: %s partial=%v author=%s\n", event.ID, event.Partial, event.Author)

		if event.UsageMetadata != nil {
			u := event.UsageMetadata
			fmt.Printf(
				"tokens prompt=%d candidates=%d thoughts=%d tool_use=%d cached=%d total=%d\n",
				u.PromptTokenCount,
				u.CandidatesTokenCount,
				u.ThoughtsTokenCount,
				u.ToolUsePromptTokenCount,
				u.CachedContentTokenCount,
				u.TotalTokenCount,
			)
		}
	}

	getResp, err := sessionService.Get(ctx, &session.GetRequest{
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

	fmt.Println(outputText)
	return nil
}
