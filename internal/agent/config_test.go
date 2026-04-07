package agent

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadConfig(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "agent.yaml")
	err := os.WriteFile(configPath, []byte(`name: browser_agent
model: gemini-3-flash-preview
description: Agent with agent-browser CLI automation tools.
instruction: crawl
`), 0o644)
	require.NoError(t, err)

	cfg, err := LoadConfig(configPath)
	require.NoError(t, err)

	assert.Equal(t, Config{
		Name:        "browser_agent",
		ModelID:     "gemini-3-flash-preview",
		Description: "Agent with agent-browser CLI automation tools.",
		Instruction: "crawl",
	}, cfg)
}

func TestLoadConfigRejectsMissingRequiredField(t *testing.T) {
	tests := []struct {
		name        string
		configText  string
		wantMessage string
	}{
		{
			name: "missing name",
			configText: `model: gemini-3-flash-preview
description: Agent with agent-browser CLI automation tools.
instruction: crawl
`,
			wantMessage: "name is required",
		},
		{
			name: "missing model",
			configText: `name: browser_agent
description: Agent with agent-browser CLI automation tools.
instruction: crawl
`,
			wantMessage: "model is required",
		},
		{
			name: "missing description",
			configText: `name: browser_agent
model: gemini-3-flash-preview
instruction: crawl
`,
			wantMessage: "description is required",
		},
		{
			name: "missing instruction",
			configText: `name: browser_agent
model: gemini-3-flash-preview
description: Agent with agent-browser CLI automation tools.
`,
			wantMessage: "instruction is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			configPath := filepath.Join(t.TempDir(), "agent.yaml")
			err := os.WriteFile(configPath, []byte(tt.configText), 0o644)
			require.NoError(t, err)

			_, err = LoadConfig(configPath)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantMessage)
		})
	}
}
