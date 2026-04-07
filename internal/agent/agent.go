package agent

import (
	agentoutput "skipjd/internal/agent/output"

	adkagent "google.golang.org/adk/agent"
	"google.golang.org/adk/agent/llmagent"
	"google.golang.org/adk/model"
	"google.golang.org/adk/tool"
	"google.golang.org/genai"
)

func NewBrowserAgent(cfg Config, modelInstance model.LLM) (adkagent.Agent, error) {
	agentBrowserToolset, err := newAgentBrowserToolset()
	if err != nil {
		return nil, err
	}

	return llmagent.New(llmagent.Config{
		Name:        cfg.Name,
		Model:       modelInstance,
		Description: cfg.Description,
		Instruction: cfg.Instruction,
		OutputKey:   agentoutput.CollectedPostingsKey,
		Toolsets:    []tool.Toolset{agentBrowserToolset},
		InputSchema: &genai.Schema{
			Type: "object",
			Properties: map[string]*genai.Schema{
				"preferred_companies": {
					Type:  "array",
					Items: &genai.Schema{Type: "string"},
				},
				"last_updated": {
					Type:   "string",
					Format: "date",
				},
				"today_date": {
					Type:   "string",
					Format: "date",
				},
			},
			Required: []string{"preferred_companies", "last_updated", "today_date"},
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
							"url": {
								Type: "string",
							},
							"closing_date": {
								Type: "string",
							},
							"min_experience_years": {
								Type: "integer",
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
