package crawler

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"google.golang.org/adk/agent"
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

type agentBrowserCommandRunner func(
	ctx context.Context,
	executable string,
	sessionName string,
	timeout time.Duration,
	args ...string,
) agentBrowserCommandResult

type agentBrowserToolset struct {
	executable  string
	sessionName string
	timeout     time.Duration
	runCommand  agentBrowserCommandRunner
	tools       []tool.Tool
}

type agentBrowserOpenArgs struct {
	URL string `json:"url"`
}

type agentBrowserSnapshotArgs struct {
	Interactive *bool   `json:"interactive,omitempty"`
	Compact     *bool   `json:"compact,omitempty"`
	Depth       *int    `json:"depth,omitempty"`
	Selector    *string `json:"selector,omitempty"`
}

type agentBrowserSelectorArgs struct {
	Selector string `json:"selector"`
}

type agentBrowserTextInputArgs struct {
	Selector string `json:"selector"`
	Text     string `json:"text"`
}

type agentBrowserWaitArgs struct {
	Selector     *string `json:"selector,omitempty"`
	Milliseconds *int    `json:"milliseconds,omitempty"`
}

type agentBrowserNoArgs struct{}

func newAgentBrowserToolset() (tool.Toolset, error) {
	return newAgentBrowserToolsetWithRunner(
		agentBrowserExecutable,
		agentBrowserSessionName,
		agentBrowserDefaultTimeout,
		runAgentBrowserCommand,
	)
}

func newAgentBrowserToolsetWithRunner(
	executable string,
	sessionName string,
	timeout time.Duration,
	runner agentBrowserCommandRunner,
) (*agentBrowserToolset, error) {
	executable = strings.TrimSpace(executable)
	if executable == "" {
		return nil, fmt.Errorf("agent-browser executable is required")
	}

	sessionName = strings.TrimSpace(sessionName)
	if sessionName == "" {
		return nil, fmt.Errorf("agent-browser session name is required")
	}

	if timeout <= 0 {
		timeout = agentBrowserDefaultTimeout
	}
	if runner == nil {
		runner = runAgentBrowserCommand
	}

	toolset := &agentBrowserToolset{
		executable:  executable,
		sessionName: sessionName,
		timeout:     timeout,
		runCommand:  runner,
	}

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

func (t *agentBrowserToolset) Tools(agent.ReadonlyContext) ([]tool.Tool, error) {
	tools := make([]tool.Tool, len(t.tools))
	copy(tools, t.tools)
	return tools, nil
}

func (t *agentBrowserToolset) buildTools() ([]tool.Tool, error) {
	toolDefs := []struct {
		name        string
		description string
		build       func() (tool.Tool, error)
	}{
		{
			name:        "agent_browser_open",
			description: "Navigate the browser session to a URL.",
			build: func() (tool.Tool, error) {
				return functiontool.New(functiontool.Config{
					Name:        "agent_browser_open",
					Description: "Navigate the browser session to a URL.",
				}, t.open)
			},
		},
		{
			name:        "agent_browser_snapshot",
			description: "Return the accessibility snapshot with @refs for interactive elements.",
			build: func() (tool.Tool, error) {
				return functiontool.New(functiontool.Config{
					Name:        "agent_browser_snapshot",
					Description: "Return the accessibility snapshot with @refs for interactive elements.",
				}, t.snapshot)
			},
		},
		{
			name:        "agent_browser_click",
			description: "Click an element by CSS selector or @ref from the latest snapshot.",
			build: func() (tool.Tool, error) {
				return functiontool.New(functiontool.Config{
					Name:        "agent_browser_click",
					Description: "Click an element by CSS selector or @ref from the latest snapshot.",
				}, t.click)
			},
		},
		{
			name:        "agent_browser_fill",
			description: "Clear an input and fill it by CSS selector or @ref.",
			build: func() (tool.Tool, error) {
				return functiontool.New(functiontool.Config{
					Name:        "agent_browser_fill",
					Description: "Clear an input and fill it by CSS selector or @ref.",
				}, t.fill)
			},
		},
		{
			name:        "agent_browser_type",
			description: "Type text into an input by CSS selector or @ref.",
			build: func() (tool.Tool, error) {
				return functiontool.New(functiontool.Config{
					Name:        "agent_browser_type",
					Description: "Type text into an input by CSS selector or @ref.",
				}, t.typeText)
			},
		},
		{
			name:        "agent_browser_wait",
			description: "Wait for a selector/@ref or a duration in milliseconds.",
			build: func() (tool.Tool, error) {
				return functiontool.New(functiontool.Config{
					Name:        "agent_browser_wait",
					Description: "Wait for a selector/@ref or a duration in milliseconds.",
				}, t.wait)
			},
		},
		{
			name:        "agent_browser_get_text",
			description: "Read text content from an element by CSS selector or @ref.",
			build: func() (tool.Tool, error) {
				return functiontool.New(functiontool.Config{
					Name:        "agent_browser_get_text",
					Description: "Read text content from an element by CSS selector or @ref.",
				}, t.getText)
			},
		},
		{
			name:        "agent_browser_get_url",
			description: "Read the current page URL.",
			build: func() (tool.Tool, error) {
				return functiontool.New(functiontool.Config{
					Name:        "agent_browser_get_url",
					Description: "Read the current page URL.",
				}, t.getURL)
			},
		},
		{
			name:        "agent_browser_close",
			description: "Close the current browser session.",
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

	return t.run(ctx, t.timeout, "open", targetURL), nil
}

func (t *agentBrowserToolset) snapshot(ctx tool.Context, input agentBrowserSnapshotArgs) (agentBrowserCommandResult, error) {
	args := []string{"snapshot"}
	if input.Interactive != nil && *input.Interactive {
		args = append(args, "--interactive")
	}
	if input.Compact != nil && *input.Compact {
		args = append(args, "--compact")
	}
	if input.Depth != nil && *input.Depth > 0 {
		args = append(args, "--depth", strconv.Itoa(*input.Depth))
	}

	selector := ""
	if input.Selector != nil {
		selector = strings.TrimSpace(*input.Selector)
	}
	if selector != "" {
		args = append(args, "--selector", selector)
	}

	result := t.run(ctx, t.timeout, args...)
	if result.Error == "" && result.ExitCode == 0 && strings.TrimSpace(result.Stdout) == "" {
		result.Error = "agent-browser snapshot returned empty output"
	}

	return result, nil
}

func (t *agentBrowserToolset) click(ctx tool.Context, input agentBrowserSelectorArgs) (agentBrowserCommandResult, error) {
	selector := strings.TrimSpace(input.Selector)
	if selector == "" {
		return newAgentBrowserInputError("selector is required"), nil
	}

	return t.run(ctx, t.timeout, "click", selector), nil
}

func (t *agentBrowserToolset) fill(ctx tool.Context, input agentBrowserTextInputArgs) (agentBrowserCommandResult, error) {
	selector := strings.TrimSpace(input.Selector)
	if selector == "" {
		return newAgentBrowserInputError("selector is required"), nil
	}

	return t.run(ctx, t.timeout, "fill", selector, input.Text), nil
}

func (t *agentBrowserToolset) typeText(ctx tool.Context, input agentBrowserTextInputArgs) (agentBrowserCommandResult, error) {
	selector := strings.TrimSpace(input.Selector)
	if selector == "" {
		return newAgentBrowserInputError("selector is required"), nil
	}

	return t.run(ctx, t.timeout, "type", selector, input.Text), nil
}

func (t *agentBrowserToolset) wait(ctx tool.Context, input agentBrowserWaitArgs) (agentBrowserCommandResult, error) {
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
		return t.run(ctx, t.timeout, "wait", selector), nil
	default:
		waitTimeout := t.timeout
		requestedWait := time.Duration(milliseconds) * time.Millisecond
		if timeoutWithMargin := requestedWait + agentBrowserWaitTimeoutMargin; timeoutWithMargin > waitTimeout {
			waitTimeout = timeoutWithMargin
		}
		return t.run(ctx, waitTimeout, "wait", strconv.Itoa(milliseconds)), nil
	}
}

func (t *agentBrowserToolset) getText(ctx tool.Context, input agentBrowserSelectorArgs) (agentBrowserCommandResult, error) {
	selector := strings.TrimSpace(input.Selector)
	if selector == "" {
		return newAgentBrowserInputError("selector is required"), nil
	}

	return t.run(ctx, t.timeout, "get", "text", selector), nil
}

func (t *agentBrowserToolset) getURL(ctx tool.Context, _ agentBrowserNoArgs) (agentBrowserCommandResult, error) {
	return t.run(ctx, t.timeout, "get", "url"), nil
}

func (t *agentBrowserToolset) close(ctx tool.Context, _ agentBrowserNoArgs) (agentBrowserCommandResult, error) {
	return t.run(ctx, t.timeout, "close"), nil
}

func (t *agentBrowserToolset) run(ctx context.Context, timeout time.Duration, args ...string) agentBrowserCommandResult {
	return t.runCommand(ctx, t.executable, t.sessionName, timeout, args...)
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
		agentBrowserExecutable,
		agentBrowserSessionName,
		agentBrowserDefaultTimeout,
		"close",
	)
}

func runAgentBrowserCommand(
	ctx context.Context,
	executable string,
	sessionName string,
	timeout time.Duration,
	args ...string,
) agentBrowserCommandResult {
	if timeout <= 0 {
		timeout = agentBrowserDefaultTimeout
	}

	commandCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	commandArgs := append([]string{"--session", sessionName}, args...)
	command := exec.CommandContext(commandCtx, executable, commandArgs...)

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
	if exitError, ok := errors.AsType[*exec.ExitError](err); ok {
		result.ExitCode = exitError.ExitCode()
	}

	if errors.Is(commandCtx.Err(), context.DeadlineExceeded) {
		result.Error = fmt.Sprintf("agent-browser command timed out after %s", timeout)
		return result
	}

	result.Error = err.Error()
	return result
}
