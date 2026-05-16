package geminiexec

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"skipjd/internal/retry"
)

// DefaultTimeout is used when CallOptions.Timeout is zero. The default is
// generous because gemini-cli occasionally cold-starts.
const DefaultTimeout = 600 * time.Second

// CallOptions bundles the per-invocation parameters for Call. Prompt and
// Input are concatenated into a single file ("@<file>" reference) so the
// model sees the prompt followed by the input body verbatim.
//
// Label is included in error / log messages to disambiguate concurrent
// calls (e.g. "posting_id=123", "user_id=1"). When DebugDir is non-empty
// the staging file is retained there instead of a tempfile.
type CallOptions struct {
	Prompt   string
	Input    string
	Model    string
	Label    string
	DebugDir string
	Timeout  time.Duration
}

// Call stages opts.Input to disk, invokes gemini-cli with opts.Prompt and
// `@<file>`, and returns the trimmed stdout content.
//
// Retried up to twice on timeout / exit-code 1/124/130. On other failures
// the underlying error is returned wrapped with opts.Label for traceability.
func Call(ctx context.Context, opts CallOptions) (string, error) {
	timeout := opts.Timeout
	if timeout <= 0 {
		timeout = DefaultTimeout
	}

	bodyDir, bodyName, cleanup, err := stageInput(opts.Input, opts.Label, opts.DebugDir)
	if err != nil {
		return "", err
	}
	if cleanup != "" {
		defer func() { _ = os.Remove(cleanup) }()
	}

	var stdout strings.Builder

	err = retry.Do(ctx, 2, 2*time.Second,
		func(attemptCtx context.Context) error {
			cmdCtx, cancel := context.WithTimeout(attemptCtx, timeout)
			defer cancel()

			cmd := exec.CommandContext(cmdCtx,
				"gemini",
				"--skip-trust",
				"-m", opts.Model,
				"-p", opts.Prompt+"\n\n@"+bodyName,
				"-o", "text",
			)
			cmd.Dir = bodyDir

			stdout.Reset()
			var stderr strings.Builder
			cmd.Stdout = &stdout
			cmd.Stderr = &stderr

			runErr := cmd.Run()
			if errors.Is(cmdCtx.Err(), context.DeadlineExceeded) {
				return fmt.Errorf("gemini timeout %s after=%s", opts.Label, timeout)
			}
			if runErr != nil {
				return classifyError(runErr, stderr.String(), opts.Label)
			}
			return nil
		},
		func(err error) bool {
			if err == nil {
				return false
			}
			if strings.Contains(err.Error(), "timeout") || errors.Is(err, context.DeadlineExceeded) {
				return true
			}
			var exitErr *exec.ExitError
			if errors.As(err, &exitErr) {
				code := exitErr.ExitCode()
				return code == 1 || code == 124 || code == 130
			}
			return false
		},
	)

	if err != nil {
		return "", err
	}

	return strings.TrimSpace(stdout.String()), nil
}

// stageInput writes input to a file gemini-cli can reference via "@<name>".
// When debugDir is set the file is kept there with a stable name based on
// label; otherwise a tempfile is used and the caller deletes it via the
// returned cleanup path.
func stageInput(input, label, debugDir string) (dir, name, cleanup string, err error) {
	if debugDir != "" {
		safe := sanitizeLabel(label)
		if safe == "" {
			safe = "input"
		}
		name = safe + ".txt"
		path := filepath.Join(debugDir, name)
		if werr := os.WriteFile(path, []byte(input), 0o600); werr != nil {
			return "", "", "", fmt.Errorf("write debug input: %w", werr)
		}
		return debugDir, name, "", nil
	}

	f, ferr := os.CreateTemp("", "geminiexec-*.txt")
	if ferr != nil {
		return "", "", "", fmt.Errorf("create tempfile: %w", ferr)
	}
	path := f.Name()
	if _, werr := f.Write([]byte(input)); werr != nil {
		_ = f.Close()
		_ = os.Remove(path)
		return "", "", "", fmt.Errorf("write tempfile: %w", werr)
	}
	if cerr := f.Close(); cerr != nil {
		_ = os.Remove(path)
		return "", "", "", fmt.Errorf("close tempfile: %w", cerr)
	}
	return filepath.Dir(path), filepath.Base(path), path, nil
}

func classifyError(err error, stderrText, label string) error {
	var execErr *exec.Error
	if errors.As(err, &execErr) && errors.Is(execErr.Err, exec.ErrNotFound) {
		return errors.New("gemini not found — install gemini-cli and ensure it is on PATH")
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return fmt.Errorf("gemini exit=%d %s stderr=%q", exitErr.ExitCode(), label, TailLines(stderrText, 3))
	}
	return fmt.Errorf("gemini run: %w", err)
}

// TailLines returns up to the last n lines of s with surrounding whitespace
// trimmed. Used to keep error messages bounded when surfacing gemini-cli
// stderr tails.
func TailLines(s string, n int) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	lines := strings.Split(s, "\n")
	if len(lines) <= n {
		return strings.Join(lines, "\n")
	}
	return strings.Join(lines[len(lines)-n:], "\n")
}

// sanitizeLabel makes label safe for use as a filename. It replaces any
// rune that isn't a letter, digit, '_' or '-' with '_'. Used only for
// debug-dir staging.
func sanitizeLabel(label string) string {
	var b strings.Builder
	b.Grow(len(label))
	for _, r := range label {
		switch {
		case r >= 'a' && r <= 'z',
			r >= 'A' && r <= 'Z',
			r >= '0' && r <= '9',
			r == '_', r == '-':
			b.WriteRune(r)
		default:
			b.WriteRune('_')
		}
	}
	return b.String()
}
