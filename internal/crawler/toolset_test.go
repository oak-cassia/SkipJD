package crawler

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/adk/agent"
	"google.golang.org/adk/memory"
	"google.golang.org/adk/session"
	"google.golang.org/adk/tool"
	"google.golang.org/adk/tool/toolconfirmation"
	"google.golang.org/genai"
)

type stubToolContext struct {
	context.Context
}

func (c stubToolContext) FunctionCallID() string { return "stub-function-call-id" }

func (c stubToolContext) Actions() *session.EventActions { return &session.EventActions{} }

func (c stubToolContext) SearchMemory(context.Context, string) (*memory.SearchResponse, error) {
	return nil, nil
}

func (c stubToolContext) ToolConfirmation() *toolconfirmation.ToolConfirmation { return nil }

func (c stubToolContext) RequestConfirmation(string, any) error { return nil }

func (c stubToolContext) Artifacts() agent.Artifacts { return nil }

func (c stubToolContext) State() session.State { return nil }

func (c stubToolContext) UserContent() *genai.Content { return nil }

func (c stubToolContext) InvocationID() string { return "stub-invocation-id" }

func (c stubToolContext) AgentName() string { return "stub-agent" }

func (c stubToolContext) ReadonlyState() session.ReadonlyState { return nil }

func (c stubToolContext) UserID() string { return "stub-user" }

func (c stubToolContext) AppName() string { return "stub-app" }

func (c stubToolContext) SessionID() string { return "stub-session" }

func (c stubToolContext) Branch() string { return "stub-branch" }

type runnableTool interface {
	tool.Tool
	Run(ctx tool.Context, args any) (map[string]any, error)
}

func TestAgentBrowserToolsetBuildsExpectedCommands(t *testing.T) {
	tests := []struct {
		name     string
		toolName string
		input    map[string]any
		wantArgs []string
	}{
		{
			name:     "open",
			toolName: "agent_browser_open",
			input:    map[string]any{"url": "https://example.com/jobs"},
			wantArgs: []string{"--session", agentBrowserSessionName, "open", "https://example.com/jobs"},
		},
		{
			name:     "snapshot",
			toolName: "agent_browser_snapshot",
			input: map[string]any{
				"interactive": true,
				"compact":     true,
				"depth":       3,
				"selector":    "#content",
			},
			wantArgs: []string{"--session", agentBrowserSessionName, "snapshot", "--interactive", "--compact", "--depth", "3", "--selector", "#content"},
		},
		{
			name:     "click",
			toolName: "agent_browser_click",
			input:    map[string]any{"selector": "@e2"},
			wantArgs: []string{"--session", agentBrowserSessionName, "click", "@e2"},
		},
		{
			name:     "fill",
			toolName: "agent_browser_fill",
			input:    map[string]any{"selector": "@e3", "text": "서버"},
			wantArgs: []string{"--session", agentBrowserSessionName, "fill", "@e3", "서버"},
		},
		{
			name:     "type",
			toolName: "agent_browser_type",
			input:    map[string]any{"selector": "@e4", "text": "AI"},
			wantArgs: []string{"--session", agentBrowserSessionName, "type", "@e4", "AI"},
		},
		{
			name:     "wait selector",
			toolName: "agent_browser_wait",
			input:    map[string]any{"selector": "@e5"},
			wantArgs: []string{"--session", agentBrowserSessionName, "wait", "@e5"},
		},
		{
			name:     "wait milliseconds",
			toolName: "agent_browser_wait",
			input:    map[string]any{"milliseconds": 4500},
			wantArgs: []string{"--session", agentBrowserSessionName, "wait", "4500"},
		},
		{
			name:     "get text",
			toolName: "agent_browser_get_text",
			input:    map[string]any{"selector": "@e6"},
			wantArgs: []string{"--session", agentBrowserSessionName, "get", "text", "@e6"},
		},
		{
			name:     "get url",
			toolName: "agent_browser_get_url",
			input:    map[string]any{},
			wantArgs: []string{"--session", agentBrowserSessionName, "get", "url"},
		},
		{
			name:     "close",
			toolName: "agent_browser_close",
			input:    map[string]any{},
			wantArgs: []string{"--session", agentBrowserSessionName, "close"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			capturePath := filepath.Join(t.TempDir(), "agent-browser-args.txt")
			t.Setenv("AGENT_BROWSER_ARGS_FILE", capturePath)
			installFakeAgentBrowser(t, `
printf '%s\n' "$@" > "$AGENT_BROWSER_ARGS_FILE"
printf 'ok\n'
`)

			toolset, err := newAgentBrowserToolset()
			require.NoError(t, err)

			commandTool := getRunnableToolByName(t, toolset, tt.toolName)
			got, err := commandTool.Run(stubToolContext{Context: context.Background()}, tt.input)
			require.NoError(t, err)

			assert.Equal(t, tt.wantArgs, readCapturedArgs(t, capturePath))
			assert.Equal(t, "ok\n", got["stdout"])
			assert.Empty(t, got["stderr"])
			assert.Equal(t, float64(0), got["exit_code"])
		})
	}
}

func TestAgentBrowserSnapshotRejectsEmptyOutput(t *testing.T) {
	installFakeAgentBrowser(t, ":")

	toolset, err := newAgentBrowserToolset()
	require.NoError(t, err)

	commandTool := getRunnableToolByName(t, toolset, "agent_browser_snapshot")
	got, err := commandTool.Run(stubToolContext{Context: context.Background()}, map[string]any{})
	require.NoError(t, err)

	assert.Equal(t, "agent-browser snapshot returned empty output", got["error"])
	assert.Equal(t, float64(0), got["exit_code"])
}

func TestRunAgentBrowserCommandReportsMissingCLI(t *testing.T) {
	t.Setenv("PATH", t.TempDir())

	result := runAgentBrowserCommand(context.Background(), agentBrowserDefaultTimeout, "snapshot")

	assert.Equal(t, -1, result.ExitCode)
	assert.Contains(t, result.Error, "executable file not found")
}

func TestRunAgentBrowserCommandReportsNonZeroExit(t *testing.T) {
	installFakeAgentBrowser(t, `
printf 'stdout-line\n'
printf 'stderr-line\n' >&2
exit 7
`)

	result := runAgentBrowserCommand(context.Background(), agentBrowserDefaultTimeout, "snapshot")

	assert.Equal(t, 7, result.ExitCode)
	assert.Equal(t, "stdout-line\n", result.Stdout)
	assert.Equal(t, "stderr-line\n", result.Stderr)
	assert.Contains(t, result.Error, "exit status 7")
}

func TestRunAgentBrowserCommandReportsTimeout(t *testing.T) {
	installFakeAgentBrowser(t, "/bin/sleep 2")

	result := runAgentBrowserCommand(context.Background(), 50, "snapshot")

	assert.Equal(t, -1, result.ExitCode)
	assert.Equal(t, "agent-browser command timed out after 50ns", result.Error)
}

func TestAgentBrowserWaitRejectsAmbiguousInput(t *testing.T) {
	installFakeAgentBrowser(t, "printf 'unexpected'")

	toolset, err := newAgentBrowserToolset()
	require.NoError(t, err)

	commandTool := getRunnableToolByName(t, toolset, "agent_browser_wait")
	got, err := commandTool.Run(
		stubToolContext{Context: context.Background()},
		map[string]any{"selector": "@e1", "milliseconds": 1000},
	)
	require.NoError(t, err)

	assert.Equal(t, "provide either selector or milliseconds, not both", got["error"])
	assert.Equal(t, float64(-1), got["exit_code"])
	assert.Empty(t, got["stdout"])
}

func installFakeAgentBrowser(t *testing.T, body string) {
	t.Helper()

	commandDir := t.TempDir()
	commandPath := filepath.Join(commandDir, agentBrowserExecutable)
	script := "#!/bin/sh\n" + strings.TrimLeft(body, "\n") + "\n"
	err := os.WriteFile(commandPath, []byte(script), 0o755)
	require.NoError(t, err)
	t.Setenv("PATH", commandDir)
}

func readCapturedArgs(t *testing.T, capturePath string) []string {
	t.Helper()

	data, err := os.ReadFile(capturePath)
	require.NoError(t, err)

	return strings.Split(strings.TrimRight(string(data), "\n"), "\n")
}

func getRunnableToolByName(t *testing.T, toolset tool.Toolset, toolName string) runnableTool {
	t.Helper()

	tools, err := toolset.Tools(nil)
	require.NoError(t, err)

	for _, toolInstance := range tools {
		if toolInstance.Name() != toolName {
			continue
		}

		commandTool, ok := toolInstance.(runnableTool)
		require.True(t, ok)
		return commandTool
	}

	require.FailNow(t, "tool not found: "+toolName)
	return nil
}
