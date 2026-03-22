package main

import (
	"context"
	"fmt"
	"log"
	"os"

	"google.golang.org/adk/agent"
	"google.golang.org/adk/model"
	"google.golang.org/adk/model/gemini"
	"google.golang.org/adk/runner"
	"google.golang.org/adk/session"
	"google.golang.org/genai"
	"gopkg.in/yaml.v3"
)

const appName = "browser_agent"
const userID = "default_user"
const sessionID = "default_session"
const defaultTargetSite = "https://www.gamejob.co.kr/Recruit/joblist?menucode=duty"

func main() {
	ctx := context.Background()

	rootAgent, err := newBrowserAgent(ctx)
	if err != nil {
		log.Fatalf("Failed to create agent: %v", err)
	}

	sessionService := session.InMemoryService()
	resp, err := sessionService.Create(ctx, &session.CreateRequest{
		AppName:   appName,
		UserID:    userID,
		SessionID: sessionID,
		State: map[string]any{
			"target_site": defaultTargetSite,
			"preferred_companies": []string{
				//"넥슨",
				//"크래프톤",
				//"엔씨소프트",
				"데브시스터즈",
				//"스마일게이트",
				//"넷마블",
			},
			"preferred_positions": []string{
				"게임 서버",
				//"AI",
				//"AX",
			},
		},
	})
	if err != nil {
		log.Fatalf("Failed to create session: %v", err)
	}

	r, err := runner.New(runner.Config{
		AppName:        appName,
		Agent:          rootAgent,
		SessionService: sessionService,
	})
	if err != nil {
		log.Fatalf("Failed to create runner: %v", err)
	}

	currentSession := resp.Session
	for _, err := range r.Run(
		ctx,
		userID,
		currentSession.ID(),
		genai.NewContentFromText("run", "user"),
		agent.RunConfig{},
	) {
		if err != nil {
			log.Fatalf("Agent run failed: %v", err)
		}
	}

	getResp, err := sessionService.Get(ctx, &session.GetRequest{
		AppName:   appName,
		UserID:    userID,
		SessionID: currentSession.ID(),
	})
	if err != nil {
		log.Fatalf("Failed to load session: %v", err)
	}

	output, err := getResp.Session.State().Get("collected_postings")
	if err != nil {
		log.Fatalf("Failed to load collected_postings from session state: %v", err)
	}

	outputText, ok := output.(string)
	if !ok {
		log.Fatalf("Unexpected collected_postings type: %T", output)
	}

	fmt.Println(outputText)
}

type AgentConfig struct {
	Name        string `yaml:"name"`
	ModelID     string `yaml:"model"`
	Description string `yaml:"description"`
	Instruction string `yaml:"instruction"`
}

func loadAgentConfig(path string) (AgentConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return AgentConfig{}, err
	}

	var cfg AgentConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return AgentConfig{}, fmt.Errorf("parse %s: %w", path, err)
	}

	if cfg.Name == "" {
		return AgentConfig{}, fmt.Errorf("%s: name is required", path)
	}
	if cfg.ModelID == "" {
		return AgentConfig{}, fmt.Errorf("%s: model is required", path)
	}
	if cfg.Description == "" {
		return AgentConfig{}, fmt.Errorf("%s: description is required", path)
	}
	if cfg.Instruction == "" {
		return AgentConfig{}, fmt.Errorf("%s: instruction is required", path)
	}

	return cfg, nil
}

func newModel(ctx context.Context, modelID string) (model.LLM, error) {
	return gemini.NewModel(ctx, modelID, &genai.ClientConfig{
		APIKey: os.Getenv("GOOGLE_API_KEY"),
	})
}
