package crawler

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

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

type capturedAgentBrowserCommand struct {
	executable  string
	sessionName string
	timeout     time.Duration
	args        []string
}

func TestAgentBrowserToolsetBuildsExpectedCommands(t *testing.T) {
	tests := []struct {
		name           string
		toolName       string
		input          map[string]any
		wantArgs       []string
		wantTimeout    time.Duration
		wantStdoutText string
	}{
		{
			name:           "open",
			toolName:       "agent_browser_open",
			input:          map[string]any{"url": "https://example.com/jobs"},
			wantArgs:       []string{"open", "https://example.com/jobs"},
			wantTimeout:    3 * time.Second,
			wantStdoutText: "ok\n",
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
			wantArgs:       []string{"snapshot", "--interactive", "--compact", "--depth", "3", "--selector", "#content"},
			wantTimeout:    3 * time.Second,
			wantStdoutText: "ok\n",
		},
		{
			name:           "click",
			toolName:       "agent_browser_click",
			input:          map[string]any{"selector": "@e2"},
			wantArgs:       []string{"click", "@e2"},
			wantTimeout:    3 * time.Second,
			wantStdoutText: "ok\n",
		},
		{
			name:           "fill",
			toolName:       "agent_browser_fill",
			input:          map[string]any{"selector": "@e3", "text": "서버"},
			wantArgs:       []string{"fill", "@e3", "서버"},
			wantTimeout:    3 * time.Second,
			wantStdoutText: "ok\n",
		},
		{
			name:           "type",
			toolName:       "agent_browser_type",
			input:          map[string]any{"selector": "@e4", "text": "AX"},
			wantArgs:       []string{"type", "@e4", "AX"},
			wantTimeout:    3 * time.Second,
			wantStdoutText: "ok\n",
		},
		{
			name:           "wait selector",
			toolName:       "agent_browser_wait",
			input:          map[string]any{"selector": "@e5"},
			wantArgs:       []string{"wait", "@e5"},
			wantTimeout:    3 * time.Second,
			wantStdoutText: "ok\n",
		},
		{
			name:           "wait milliseconds",
			toolName:       "agent_browser_wait",
			input:          map[string]any{"milliseconds": 4500},
			wantArgs:       []string{"wait", "4500"},
			wantTimeout:    9500 * time.Millisecond,
			wantStdoutText: "ok\n",
		},
		{
			name:           "get text",
			toolName:       "agent_browser_get_text",
			input:          map[string]any{"selector": "@e6"},
			wantArgs:       []string{"get", "text", "@e6"},
			wantTimeout:    3 * time.Second,
			wantStdoutText: "ok\n",
		},
		{
			name:           "get url",
			toolName:       "agent_browser_get_url",
			input:          map[string]any{},
			wantArgs:       []string{"get", "url"},
			wantTimeout:    3 * time.Second,
			wantStdoutText: "ok\n",
		},
		{
			name:           "close",
			toolName:       "agent_browser_close",
			input:          map[string]any{},
			wantArgs:       []string{"close"},
			wantTimeout:    3 * time.Second,
			wantStdoutText: "ok\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var captured capturedAgentBrowserCommand
			toolset, err := newAgentBrowserToolsetWithRunner(
				"/usr/local/bin/agent-browser",
				"skipjd-test-session",
				3*time.Second,
				func(ctx context.Context, executable string, sessionName string, timeout time.Duration, args ...string) agentBrowserCommandResult {
					_ = ctx
					captured = capturedAgentBrowserCommand{
						executable:  executable,
						sessionName: sessionName,
						timeout:     timeout,
						args:        slices.Clone(args),
					}
					return agentBrowserCommandResult{Stdout: "ok\n"}
				},
			)
			require.NoError(t, err)

			commandTool := getRunnableToolByName(t, toolset, tt.toolName)
			got, err := commandTool.Run(stubToolContext{Context: context.Background()}, tt.input)
			require.NoError(t, err)

			assert.Equal(t, "/usr/local/bin/agent-browser", captured.executable)
			assert.Equal(t, "skipjd-test-session", captured.sessionName)
			assert.Equal(t, tt.wantTimeout, captured.timeout)
			assert.Equal(t, tt.wantArgs, captured.args)
			assert.Equal(t, tt.wantStdoutText, got["stdout"])
			assert.Empty(t, got["stderr"])
			assert.Equal(t, float64(0), got["exit_code"])
		})
	}
}

func TestAgentBrowserSnapshotRejectsEmptyOutput(t *testing.T) {
	toolset, err := newAgentBrowserToolsetWithRunner(
		"agent-browser",
		"skipjd-test-session",
		3*time.Second,
		func(ctx context.Context, executable string, sessionName string, timeout time.Duration, args ...string) agentBrowserCommandResult {
			_ = ctx
			_ = executable
			_ = sessionName
			_ = timeout
			_ = args
			return agentBrowserCommandResult{}
		},
	)
	require.NoError(t, err)

	commandTool := getRunnableToolByName(t, toolset, "agent_browser_snapshot")
	got, err := commandTool.Run(stubToolContext{Context: context.Background()}, map[string]any{})
	require.NoError(t, err)

	assert.Equal(t, "agent-browser snapshot returned empty output", got["error"])
	assert.Equal(t, float64(0), got["exit_code"])
}

func TestRunAgentBrowserCommandReportsMissingCLI(t *testing.T) {
	result := runAgentBrowserCommand(
		context.Background(),
		filepath.Join(t.TempDir(), "missing-agent-browser"),
		"skipjd-test-session",
		time.Second,
		"snapshot",
	)

	assert.Equal(t, -1, result.ExitCode)
	assert.Contains(t, result.Error, "no such file")
}

func TestRunAgentBrowserCommandReportsNonZeroExit(t *testing.T) {
	scriptPath := filepath.Join(t.TempDir(), "fake-agent-browser")
	err := os.WriteFile(scriptPath, []byte("#!/bin/sh\necho stdout-line\necho stderr-line >&2\nexit 7\n"), 0o755)
	require.NoError(t, err)

	result := runAgentBrowserCommand(
		context.Background(),
		scriptPath,
		"skipjd-test-session",
		time.Second,
		"snapshot",
	)

	assert.Equal(t, 7, result.ExitCode)
	assert.Equal(t, "stdout-line\n", result.Stdout)
	assert.Equal(t, "stderr-line\n", result.Stderr)
	assert.Contains(t, result.Error, "exit status 7")
}

func TestRunAgentBrowserCommandReportsTimeout(t *testing.T) {
	scriptPath := filepath.Join(t.TempDir(), "slow-agent-browser")
	err := os.WriteFile(scriptPath, []byte("#!/bin/sh\nsleep 2\n"), 0o755)
	require.NoError(t, err)

	result := runAgentBrowserCommand(
		context.Background(),
		scriptPath,
		"skipjd-test-session",
		50*time.Millisecond,
		"snapshot",
	)

	assert.Equal(t, -1, result.ExitCode)
	assert.Equal(t, "agent-browser command timed out after 50ms", result.Error)
}

func TestAgentBrowserWaitRejectsAmbiguousInput(t *testing.T) {
	toolset, err := newAgentBrowserToolsetWithRunner(
		"agent-browser",
		"skipjd-test-session",
		3*time.Second,
		func(ctx context.Context, executable string, sessionName string, timeout time.Duration, args ...string) agentBrowserCommandResult {
			_ = ctx
			_ = executable
			_ = sessionName
			_ = timeout
			_ = args
			return agentBrowserCommandResult{Stdout: "unexpected"}
		},
	)
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
