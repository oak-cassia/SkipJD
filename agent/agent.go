package main

import (
	"context"

	"google.golang.org/adk/agent"
	"google.golang.org/adk/agent/llmagent"
	"google.golang.org/adk/tool"
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
		Toolsets:    []tool.Toolset{playwrightToolset},
	})
}
