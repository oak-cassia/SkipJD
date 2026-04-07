package agent

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	adkagent "google.golang.org/adk/agent"
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

func (c stubToolContext) Artifacts() adkagent.Artifacts { return nil }

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
			name:     "evaluate",
			toolName: "agent_browser_evaluate",
			input:    map[string]any{"script": "document.title"},
			wantArgs: []string{"--session", agentBrowserSessionName, "eval", "document.title"},
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

func TestAgentBrowserToolsetExposesOnlyCrawlerCommands(t *testing.T) {
	toolset, err := newAgentBrowserToolset()
	require.NoError(t, err)

	tools, err := toolset.Tools(nil)
	require.NoError(t, err)

	names := make([]string, 0, len(tools))
	for _, toolInstance := range tools {
		names = append(names, toolInstance.Name())
	}

	assert.Equal(t, []string{
		"agent_browser_open",
		"agent_browser_wait",
		"agent_browser_evaluate",
		"agent_browser_close",
	}, names)
}

func TestRunAgentBrowserCommandReportsMissingCLI(t *testing.T) {
	t.Setenv("PATH", t.TempDir())

	result := runAgentBrowserCommand(context.Background(), agentBrowserDefaultTimeout, "eval", "document.title")

	assert.Equal(t, -1, result.ExitCode)
	assert.Contains(t, result.Error, "executable file not found")
}

func TestRunAgentBrowserCommandReportsNonZeroExit(t *testing.T) {
	installFakeAgentBrowser(t, `
printf 'stdout-line\n'
printf 'stderr-line\n' >&2
exit 7
`)

	result := runAgentBrowserCommand(context.Background(), agentBrowserDefaultTimeout, "eval", "document.title")

	assert.Equal(t, 7, result.ExitCode)
	assert.Equal(t, "stdout-line\n", result.Stdout)
	assert.Equal(t, "stderr-line\n", result.Stderr)
	assert.Contains(t, result.Error, "exit status 7")
}

func TestRunAgentBrowserCommandReportsTimeout(t *testing.T) {
	installFakeAgentBrowser(t, "/bin/sleep 2")

	result := runAgentBrowserCommand(context.Background(), 50, "eval", "document.title")

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

func TestAgentBrowserEvaluateRejectsEmptyScript(t *testing.T) {
	installFakeAgentBrowser(t, "printf 'unexpected'")

	toolset, err := newAgentBrowserToolset()
	require.NoError(t, err)

	commandTool := getRunnableToolByName(t, toolset, "agent_browser_evaluate")
	got, err := commandTool.Run(stubToolContext{Context: context.Background()}, map[string]any{"script": ""})
	require.NoError(t, err)

	assert.Equal(t, "script is required", got["error"])
	assert.Equal(t, float64(-1), got["exit_code"])
	assert.Empty(t, got["stdout"])
}

func TestAgentBrowserCloseRejectsRepeatedCalls(t *testing.T) {
	installFakeAgentBrowser(t, "printf 'closed\n'")

	toolset, err := newAgentBrowserToolset()
	require.NoError(t, err)

	commandTool := getRunnableToolByName(t, toolset, "agent_browser_close")

	first, err := commandTool.Run(stubToolContext{Context: context.Background()}, map[string]any{})
	require.NoError(t, err)
	assert.Equal(t, "closed\n", first["stdout"])

	second, err := commandTool.Run(stubToolContext{Context: context.Background()}, map[string]any{})
	require.NoError(t, err)
	assert.Equal(t, "browser session already closed; return final JSON now without more tool calls", second["error"])
	assert.Equal(t, float64(-1), second["exit_code"])
}

func TestAgentBrowserBlocksFurtherNavigationAfterCutoffReached(t *testing.T) {
	installFakeAgentBrowser(t, `
if [ "$3" = "eval" ]; then
  printf '[{"is_before_cutoff":true}]'
  exit 0
fi
printf 'ok\n'
`)

	toolset, err := newAgentBrowserToolset()
	require.NoError(t, err)

	openTool := getRunnableToolByName(t, toolset, "agent_browser_open")
	_, err = openTool.Run(stubToolContext{Context: context.Background()}, map[string]any{"url": "https://example.com/jobs"})
	require.NoError(t, err)

	evaluateTool := getRunnableToolByName(t, toolset, "agent_browser_evaluate")
	evaluateResult, err := evaluateTool.Run(stubToolContext{Context: context.Background()}, map[string]any{"script": "[]"})
	require.NoError(t, err)
	assert.Equal(t, float64(0), evaluateResult["exit_code"])

	waitTool := getRunnableToolByName(t, toolset, "agent_browser_wait")
	waitResult, err := waitTool.Run(stubToolContext{Context: context.Background()}, map[string]any{"milliseconds": 1000})
	require.NoError(t, err)
	assert.Equal(t, "cutoff reached; do not browse further, call agent_browser_close and return final JSON now", waitResult["error"])
	assert.Equal(t, float64(-1), waitResult["exit_code"])
}

func TestAgentBrowserOpenRejectsReopenInSameRun(t *testing.T) {
	installFakeAgentBrowser(t, "printf 'ok\n'")

	toolset, err := newAgentBrowserToolset()
	require.NoError(t, err)

	openTool := getRunnableToolByName(t, toolset, "agent_browser_open")
	_, err = openTool.Run(stubToolContext{Context: context.Background()}, map[string]any{"url": "https://example.com/jobs"})
	require.NoError(t, err)

	reopenResult, err := openTool.Run(stubToolContext{Context: context.Background()}, map[string]any{"url": "https://example.com/jobs?page=2"})
	require.NoError(t, err)
	assert.Equal(t, "browser session already opened; do not reopen listing pages in the same run", reopenResult["error"])
	assert.Equal(t, float64(-1), reopenResult["exit_code"])
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
