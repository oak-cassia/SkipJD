package agent

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Name        string `yaml:"name"`
	ModelID     string `yaml:"model"`
	Description string `yaml:"description"`
	Instruction string `yaml:"instruction"`
}

func LoadConfig(path string) (Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Config{}, err
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return Config{}, fmt.Errorf("parse %s: %w", path, err)
	}

	if cfg.Name == "" {
		return Config{}, fmt.Errorf("%s: name is required", path)
	}
	if cfg.ModelID == "" {
		return Config{}, fmt.Errorf("%s: model is required", path)
	}
	if cfg.Description == "" {
		return Config{}, fmt.Errorf("%s: description is required", path)
	}
	if cfg.Instruction == "" {
		return Config{}, fmt.Errorf("%s: instruction is required", path)
	}

	return cfg, nil
}
