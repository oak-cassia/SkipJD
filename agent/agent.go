package main

import (
	"context"
	"log"
	"os"
	"os/exec"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"google.golang.org/adk/agent"
	"google.golang.org/adk/agent/llmagent"
	"google.golang.org/adk/cmd/launcher"
	"google.golang.org/adk/cmd/launcher/full"
	"google.golang.org/adk/model/gemini"
	"google.golang.org/adk/tool"
	"google.golang.org/adk/tool/mcptoolset"
	"google.golang.org/genai"
)

func main() {
	ctx := context.Background()

	model, err := gemini.NewModel(ctx, "gemini-3.1-flash-lite-preview", &genai.ClientConfig{
		APIKey: os.Getenv("GOOGLE_API_KEY"),
	})
	if err != nil {
		log.Fatalf("Failed to create model: %v", err)
	}

	playwrightToolset, err := mcptoolset.New(mcptoolset.Config{
		Transport: &mcp.CommandTransport{
			Command: exec.Command("npx", "-y", "@playwright/mcp@latest", "--headless", "--isolated"),
		},
	})
	if err != nil {
		log.Fatalf("Failed to create Playwright MCP tool set: %v", err)
	}

	browserAgent, err := llmagent.New(llmagent.Config{
		Name:        "browser_agent",
		Model:       model,
		Description: "Agent with Playwright MCP browser automation tools.",
		Instruction: "Use the Playwright MCP tools to browse websites, inspect pages, and interact with web content.",
		Toolsets:    []tool.Toolset{playwrightToolset},
	})
	if err != nil {
		log.Fatalf("Failed to create agent: %v", err)
	}

	config := &launcher.Config{
		AgentLoader: agent.NewSingleLoader(browserAgent),
	}

	l := full.NewLauncher()
	if err = l.Execute(ctx, config, os.Args[1:]); err != nil {
		log.Fatalf("Run failed: %v\n\n%s", err, l.CommandLineSyntax())
	}
}
