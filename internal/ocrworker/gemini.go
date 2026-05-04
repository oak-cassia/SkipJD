package ocrworker

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

const geminiTimeout = 600 * time.Second

const ocrPrompt = "이 이미지에 보이는 텍스트(한국어/영어 포함)를 모두 추출해서 " +
	"줄 단위로 한 줄에 하나씩 출력해줘. 설명, 마크다운, 메타정보 없이 텍스트만. " +
	"표는 줄 단위로 풀어서 적어줘."

// ocrPayload writes the image bytes to a file (debugDir if set, otherwise a
// tempfile) then invokes gemini-cli with the OCR prompt. Returns the
// extracted text trimmed of surrounding whitespace.
//
// When debugDir is set, the image and the resulting OCR text are persisted
// as <postingID>_<idx>.{jpg,txt} for after-the-fact inspection.
func ocrPayload(ctx context.Context, payload []byte, postingID uint, idx int, debugDir string) (string, error) {
	imgDir, imgName, cleanup, err := stageImage(payload, postingID, idx, debugDir)
	if err != nil {
		return "", err
	}
	if cleanup != "" {
		defer func() { _ = os.Remove(cleanup) }()
	}

	text, err := runGemini(ctx, imgDir, imgName)
	if err != nil {
		return "", err
	}

	if debugDir != "" {
		txtPath := filepath.Join(debugDir, fmt.Sprintf("%d_%d.txt", postingID, idx))
		if err := os.WriteFile(txtPath, []byte(text), 0o600); err != nil {
			return "", fmt.Errorf("write debug text: %w", err)
		}
	}

	return text, nil
}

// stageImage writes the payload to a file gemini-cli can read via a relative
// "@<name>" reference (so cwd must be the file's directory). When debugDir
// is non-empty, the file is kept; otherwise a tempfile is created and the
// caller is given a cleanup path to remove on return.
func stageImage(payload []byte, postingID uint, idx int, debugDir string) (string, string, string, error) {
	if debugDir != "" {
		name := fmt.Sprintf("%d_%d.jpg", postingID, idx)
		path := filepath.Join(debugDir, name)
		if err := os.WriteFile(path, payload, 0o600); err != nil {
			return "", "", "", fmt.Errorf("write debug image: %w", err)
		}
		return debugDir, name, "", nil
	}

	f, err := os.CreateTemp("", "ocr-*.jpg")
	if err != nil {
		return "", "", "", fmt.Errorf("create tempfile: %w", err)
	}
	path := f.Name()
	if _, err := f.Write(payload); err != nil {
		_ = f.Close()
		_ = os.Remove(path)
		return "", "", "", fmt.Errorf("write tempfile: %w", err)
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(path)
		return "", "", "", fmt.Errorf("close tempfile: %w", err)
	}
	return filepath.Dir(path), filepath.Base(path), path, nil
}

func runGemini(ctx context.Context, cwd, imgName string) (string, error) {
	cmdCtx, cancel := context.WithTimeout(ctx, geminiTimeout)
	defer cancel()

	cmd := exec.CommandContext(cmdCtx, "gemini", "-p", ocrPrompt+" @"+imgName, "-o", "text")
	cmd.Dir = cwd

	var stdout strings.Builder
	var stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	if errors.Is(cmdCtx.Err(), context.DeadlineExceeded) {
		return "", fmt.Errorf("gemini timeout after %s img=%s", geminiTimeout, imgName)
	}
	if err != nil {
		return "", classifyGeminiError(err, stderr.String(), imgName)
	}

	return strings.TrimSpace(stdout.String()), nil
}

func classifyGeminiError(err error, stderrText, imgName string) error {
	var execErr *exec.Error
	if errors.As(err, &execErr) && errors.Is(execErr.Err, exec.ErrNotFound) {
		return errors.New("gemini not found — install gemini-cli and ensure it is on PATH")
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return fmt.Errorf("gemini exit=%d img=%s stderr=%q", exitErr.ExitCode(), imgName, tailLines(stderrText, 3))
	}
	return fmt.Errorf("gemini run: %w", err)
}

func tailLines(s string, n int) string {
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
