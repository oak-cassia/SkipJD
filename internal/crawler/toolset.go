package crawler

import (
	"os/exec"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"google.golang.org/adk/tool"
	"google.golang.org/adk/tool/mcptoolset"
)

func newPlaywrightToolset() (tool.Toolset, error) {
	return mcptoolset.New(mcptoolset.Config{
		Transport: &mcp.CommandTransport{
			Command: exec.Command("npx", "-y", "@playwright/mcp@latest", "--headless", "--isolated"),
		},
	})
}
