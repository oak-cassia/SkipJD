package crawler

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

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
