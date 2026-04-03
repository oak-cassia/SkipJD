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

type agentBrowserToolset struct {
	tools []tool.Tool
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

func (t *agentBrowserToolset) Tools(agent.ReadonlyContext) ([]tool.Tool, error) {
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
			name: "agent_browser_snapshot",
			build: func() (tool.Tool, error) {
				return functiontool.New(functiontool.Config{
					Name:        "agent_browser_snapshot",
					Description: "Return the accessibility snapshot with @refs for interactive elements.",
				}, t.snapshot)
			},
		},
		{
			name: "agent_browser_click",
			build: func() (tool.Tool, error) {
				return functiontool.New(functiontool.Config{
					Name:        "agent_browser_click",
					Description: "Click an element by CSS selector or @ref from the latest snapshot.",
				}, t.click)
			},
		},
		{
			name: "agent_browser_fill",
			build: func() (tool.Tool, error) {
				return functiontool.New(functiontool.Config{
					Name:        "agent_browser_fill",
					Description: "Clear an input and fill it by CSS selector or @ref.",
				}, t.fill)
			},
		},
		{
			name: "agent_browser_type",
			build: func() (tool.Tool, error) {
				return functiontool.New(functiontool.Config{
					Name:        "agent_browser_type",
					Description: "Type text into an input by CSS selector or @ref.",
				}, t.typeText)
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
			name: "agent_browser_get_text",
			build: func() (tool.Tool, error) {
				return functiontool.New(functiontool.Config{
					Name:        "agent_browser_get_text",
					Description: "Read text content from an element by CSS selector or @ref.",
				}, t.getText)
			},
		},
		{
			name: "agent_browser_get_url",
			build: func() (tool.Tool, error) {
				return functiontool.New(functiontool.Config{
					Name:        "agent_browser_get_url",
					Description: "Read the current page URL.",
				}, t.getURL)
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

	return runAgentBrowserCommand(ctx, agentBrowserDefaultTimeout, "open", targetURL), nil
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

	result := runAgentBrowserCommand(ctx, agentBrowserDefaultTimeout, args...)
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

	return runAgentBrowserCommand(ctx, agentBrowserDefaultTimeout, "click", selector), nil
}

func (t *agentBrowserToolset) fill(ctx tool.Context, input agentBrowserTextInputArgs) (agentBrowserCommandResult, error) {
	selector := strings.TrimSpace(input.Selector)
	if selector == "" {
		return newAgentBrowserInputError("selector is required"), nil
	}

	return runAgentBrowserCommand(ctx, agentBrowserDefaultTimeout, "fill", selector, input.Text), nil
}

func (t *agentBrowserToolset) typeText(ctx tool.Context, input agentBrowserTextInputArgs) (agentBrowserCommandResult, error) {
	selector := strings.TrimSpace(input.Selector)
	if selector == "" {
		return newAgentBrowserInputError("selector is required"), nil
	}

	return runAgentBrowserCommand(ctx, agentBrowserDefaultTimeout, "type", selector, input.Text), nil
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

func (t *agentBrowserToolset) getText(ctx tool.Context, input agentBrowserSelectorArgs) (agentBrowserCommandResult, error) {
	selector := strings.TrimSpace(input.Selector)
	if selector == "" {
		return newAgentBrowserInputError("selector is required"), nil
	}

	return runAgentBrowserCommand(ctx, agentBrowserDefaultTimeout, "get", "text", selector), nil
}

func (t *agentBrowserToolset) getURL(ctx tool.Context, _ agentBrowserNoArgs) (agentBrowserCommandResult, error) {
	return runAgentBrowserCommand(ctx, agentBrowserDefaultTimeout, "get", "url"), nil
}

func (t *agentBrowserToolset) close(ctx tool.Context, _ agentBrowserNoArgs) (agentBrowserCommandResult, error) {
	return runAgentBrowserCommand(ctx, agentBrowserDefaultTimeout, "close"), nil
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
