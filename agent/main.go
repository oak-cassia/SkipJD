package main

import (
	"context"
	"fmt"
	"log"
	"os"

	"google.golang.org/adk/agent"
	"google.golang.org/adk/cmd/launcher"
	"google.golang.org/adk/cmd/launcher/full"
	"google.golang.org/adk/model"
	"google.golang.org/adk/model/gemini"
	"google.golang.org/genai"
	"gopkg.in/yaml.v3"
)

func main() {
	ctx := context.Background()

	rootAgent, err := newBrowserAgent(ctx)
	if err != nil {
		log.Fatalf("Failed to create agent: %v", err)
	}

	config := &launcher.Config{
		AgentLoader: agent.NewSingleLoader(rootAgent),
	}

	l := full.NewLauncher()
	if err = l.Execute(ctx, config, os.Args[1:]); err != nil {
		log.Fatalf("Run failed: %v\n\n%s", err, l.CommandLineSyntax())
	}
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
