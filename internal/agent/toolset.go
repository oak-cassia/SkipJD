package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"

	adkagent "google.golang.org/adk/agent"
	"google.golang.org/adk/tool"
	"google.golang.org/adk/tool/functiontool"
)

const agentBrowserExecutable = "agent-browser"
const agentBrowserSessionName = "skipjd-browser-agent"
const agentBrowserDefaultTimeout = 25 * time.Second
const agentBrowserWaitTimeoutMargin = 5 * time.Second

type agentBrowserCommandResult struct {
	Stdout   string `json:"stdout"`
	Stderr   string `json:"stderr"`
	ExitCode int    `json:"exit_code"`
	Error    string `json:"error,omitempty"`
}

type agentBrowserToolset struct {
	mu            sync.Mutex
	started       bool
	closed        bool
	cutoffReached bool
	tools         []tool.Tool
}

type agentBrowserOpenArgs struct {
	URL string `json:"url"`
}

type agentBrowserWaitArgs struct {
	Selector     *string `json:"selector,omitempty"`
	Milliseconds *int    `json:"milliseconds,omitempty"`
}

type agentBrowserEvaluateArgs struct {
	Script string `json:"script"`
}

type agentBrowserNoArgs struct{}

func newAgentBrowserToolset() (tool.Toolset, error) {
	toolset := &agentBrowserToolset{}

	var err error
	toolset.tools, err = toolset.buildTools()
	if err != nil {
		return nil, err
	}

	return toolset, nil
}

func (t *agentBrowserToolset) Name() string {
	return "agent_browser"
}

func (t *agentBrowserToolset) Tools(adkagent.ReadonlyContext) ([]tool.Tool, error) {
	return t.tools, nil
}

func (t *agentBrowserToolset) buildTools() ([]tool.Tool, error) {
	toolDefs := []struct {
		name  string
		build func() (tool.Tool, error)
	}{
		{
			name: "agent_browser_open",
			build: func() (tool.Tool, error) {
				return functiontool.New(functiontool.Config{
					Name:        "agent_browser_open",
					Description: "Navigate the browser session to a URL.",
				}, t.open)
			},
		},
		{
			name: "agent_browser_wait",
			build: func() (tool.Tool, error) {
				return functiontool.New(functiontool.Config{
					Name:        "agent_browser_wait",
					Description: "Wait for a selector/@ref or a duration in milliseconds.",
				}, t.wait)
			},
		},
		{
			name: "agent_browser_evaluate",
			build: func() (tool.Tool, error) {
				return functiontool.New(functiontool.Config{
					Name:        "agent_browser_evaluate",
					Description: "Run JavaScript in the current page and return the result.",
				}, t.evaluate)
			},
		},
		{
			name: "agent_browser_close",
			build: func() (tool.Tool, error) {
				return functiontool.New(functiontool.Config{
					Name:        "agent_browser_close",
					Description: "Close the current browser session.",
				}, t.close)
			},
		},
	}

	tools := make([]tool.Tool, 0, len(toolDefs))
	for _, toolDef := range toolDefs {
		builtTool, err := toolDef.build()
		if err != nil {
			return nil, fmt.Errorf("create %s tool: %w", toolDef.name, err)
		}
		tools = append(tools, builtTool)
	}

	return tools, nil
}

func (t *agentBrowserToolset) open(ctx tool.Context, input agentBrowserOpenArgs) (agentBrowserCommandResult, error) {
	targetURL := strings.TrimSpace(input.URL)
	if targetURL == "" {
		return newAgentBrowserInputError("url is required"), nil
	}

	t.mu.Lock()
	switch {
	case t.closed:
		t.mu.Unlock()
		return newAgentBrowserInputError("browser session already closed; return final JSON now without more tool calls"), nil
	case t.started:
		t.mu.Unlock()
		return newAgentBrowserInputError("browser session already opened; do not reopen listing pages in the same run"), nil
	}
	t.mu.Unlock()

	result := runAgentBrowserCommand(ctx, agentBrowserDefaultTimeout, "open", targetURL)
	if result.ExitCode == 0 && result.Error == "" {
		t.mu.Lock()
		t.started = true
		t.closed = false
		t.cutoffReached = false
		t.mu.Unlock()
	}

	return result, nil
}

func (t *agentBrowserToolset) wait(ctx tool.Context, input agentBrowserWaitArgs) (agentBrowserCommandResult, error) {
	if result, blocked := t.navigationBlockedResult(); blocked {
		return result, nil
	}

	selector := ""
	if input.Selector != nil {
		selector = strings.TrimSpace(*input.Selector)
	}

	milliseconds := 0
	if input.Milliseconds != nil {
		milliseconds = *input.Milliseconds
	}

	switch {
	case selector != "" && milliseconds > 0:
		return newAgentBrowserInputError("provide either selector or milliseconds, not both"), nil
	case selector == "" && milliseconds <= 0:
		return newAgentBrowserInputError("either selector or milliseconds is required"), nil
	case selector != "":
		return runAgentBrowserCommand(ctx, agentBrowserDefaultTimeout, "wait", selector), nil
	default:
		waitTimeout := agentBrowserDefaultTimeout
		requestedWait := time.Duration(milliseconds) * time.Millisecond
		if timeoutWithMargin := requestedWait + agentBrowserWaitTimeoutMargin; timeoutWithMargin > waitTimeout {
			waitTimeout = timeoutWithMargin
		}
		return runAgentBrowserCommand(ctx, waitTimeout, "wait", strconv.Itoa(milliseconds)), nil
	}
}

func (t *agentBrowserToolset) evaluate(ctx tool.Context, input agentBrowserEvaluateArgs) (agentBrowserCommandResult, error) {
	if result, blocked := t.navigationBlockedResult(); blocked {
		return result, nil
	}

	script := strings.TrimSpace(input.Script)
	if script == "" {
		return newAgentBrowserInputError("script is required"), nil
	}

	result := runAgentBrowserCommand(ctx, agentBrowserDefaultTimeout, "eval", script)
	if result.ExitCode == 0 && result.Error == "" {
		t.noteCutoffReachedFromEvaluate(result.Stdout)
	}

	return result, nil
}

func (t *agentBrowserToolset) close(ctx tool.Context, _ agentBrowserNoArgs) (agentBrowserCommandResult, error) {
	t.mu.Lock()
	if t.closed {
		t.mu.Unlock()
		return newAgentBrowserInputError("browser session already closed; return final JSON now without more tool calls"), nil
	}
	t.closed = true
	t.mu.Unlock()

	result := runAgentBrowserCommand(ctx, agentBrowserDefaultTimeout, "close")
	if result.ExitCode != 0 || result.Error != "" {
		t.mu.Lock()
		t.closed = false
		t.mu.Unlock()
		return result, nil
	}

	if strings.TrimSpace(result.Stdout) == "" {
		result.Stdout = "browser closed; return final JSON now without more tool calls"
	}

	return result, nil
}

func (t *agentBrowserToolset) navigationBlockedResult() (agentBrowserCommandResult, bool) {
	t.mu.Lock()
	defer t.mu.Unlock()

	switch {
	case t.closed:
		return newAgentBrowserInputError("browser session already closed; return final JSON now without more tool calls"), true
	case t.cutoffReached:
		return newAgentBrowserInputError("cutoff reached; do not browse further, call agent_browser_close and return final JSON now"), true
	default:
		return agentBrowserCommandResult{}, false
	}
}

func (t *agentBrowserToolset) noteCutoffReachedFromEvaluate(stdout string) {
	stdout = strings.TrimSpace(stdout)
	if stdout == "" {
		return
	}

	var rows []map[string]any
	if err := json.Unmarshal([]byte(stdout), &rows); err != nil {
		return
	}

	for _, row := range rows {
		if rowBeforeCutoff(row) {
			t.mu.Lock()
			t.cutoffReached = true
			t.mu.Unlock()
			return
		}
	}
}

func rowBeforeCutoff(row map[string]any) bool {
	value, ok := row["is_before_cutoff"]
	if !ok || value == nil {
		return false
	}

	switch v := value.(type) {
	case bool:
		return v
	case string:
		parsed, err := strconv.ParseBool(strings.TrimSpace(v))
		return err == nil && parsed
	default:
		return false
	}
}

func newAgentBrowserInputError(message string) agentBrowserCommandResult {
	return agentBrowserCommandResult{
		ExitCode: -1,
		Error:    message,
	}
}

func closeAgentBrowserSession(ctx context.Context) agentBrowserCommandResult {
	return runAgentBrowserCommand(
		ctx,
		agentBrowserDefaultTimeout,
		"close",
	)
}

func runAgentBrowserCommand(
	ctx context.Context,
	timeout time.Duration,
	args ...string,
) agentBrowserCommandResult {
	if timeout <= 0 {
		timeout = agentBrowserDefaultTimeout
	}

	commandCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	commandArgs := append([]string{"--session", agentBrowserSessionName}, args...)
	command := exec.CommandContext(commandCtx, agentBrowserExecutable, commandArgs...)

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	command.Stdout = &stdout
	command.Stderr = &stderr

	result := agentBrowserCommandResult{
		Stdout: stdout.String(),
		Stderr: stderr.String(),
	}

	err := command.Run()
	result.Stdout = stdout.String()
	result.Stderr = stderr.String()
	if err == nil {
		return result
	}

	result.ExitCode = -1
	var exitError *exec.ExitError
	if errors.As(err, &exitError) {
		result.ExitCode = exitError.ExitCode()
	}

	if errors.Is(commandCtx.Err(), context.DeadlineExceeded) {
		result.Error = fmt.Sprintf("agent-browser command timed out after %s", timeout)
		return result
	}

	result.Error = err.Error()
	return result
}
