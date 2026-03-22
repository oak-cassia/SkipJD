package main

import (
	"context"

	"google.golang.org/adk/agent"
	"google.golang.org/adk/agent/llmagent"
	"google.golang.org/adk/tool"
	"google.golang.org/genai"
)

const agentConfigPath = "agent.yaml"

func newBrowserAgent(ctx context.Context) (agent.Agent, error) {
	cfg, err := loadAgentConfig(agentConfigPath)
	if err != nil {
		return nil, err
	}

	modelInstance, err := newModel(ctx, cfg.ModelID)
	if err != nil {
		return nil, err
	}

	playwrightToolset, err := newPlaywrightToolset()
	if err != nil {
		return nil, err
	}

	return llmagent.New(llmagent.Config{
		Name:        cfg.Name,
		Model:       modelInstance,
		Description: cfg.Description,
		Instruction: cfg.Instruction,
		OutputKey:   "collected_postings",
		Toolsets:    []tool.Toolset{playwrightToolset},
		InputSchema: &genai.Schema{
			Type: "object",
			Properties: map[string]*genai.Schema{
				"target_site": {
					Type: "string",
				},
				"preferred_companies": {
					Type:  "array",
					Items: &genai.Schema{Type: "string"},
				},
				"preferred_positions": {
					Type:  "array",
					Items: &genai.Schema{Type: "string"},
				},
				"last_updated": {
					Type:   "string",
					Format: "date-time",
				},
			},
			Required: []string{"target_site", "preferred_companies", "preferred_positions"},
		},
		OutputSchema: &genai.Schema{
			Type: "object",
			Properties: map[string]*genai.Schema{
				"postings": {
					Type: "array",
					Items: &genai.Schema{
						Type: "object",
						Properties: map[string]*genai.Schema{
							"title": {
								Type: "string",
							},
							"company": {
								Type: "string",
							},
							"closing_date": {
								Type: "string",
							},
							"url": {
								Type: "string",
							},
						},
						Required: []string{"title", "company", "closing_date", "url"},
					},
				},
			},
			Required: []string{"postings"},
		},
	})
}
