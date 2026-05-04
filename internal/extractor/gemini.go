package extractor

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

const promptTemplate = `너는 한국어 채용공고에서 지원자에게 요구되는 항목을 추출해 분류하는 도우미다.
입력은 채용공고 본문 텍스트다. 다음 3개 키만 가진 JSON 객체 하나만 출력하라:

- "experience": 요구/우대 경력에 해당하는 짧은 항목들의 배열 (예: "Unity 3년 이상", "모바일 라이브 서비스 운영 경험")
- "competency": 보유 기술/스킬/도구/언어 항목들의 배열 (예: "C# 숙련", "Git 협업")
- "trait": 인성/태도/성향 항목들의 배열 (예: "주도적인 문제 해결", "원활한 커뮤니케이션")

규칙:
- 본문에 명시되지 않은 사항은 추정하지 말 것.
- 각 항목은 한 줄짜리 짧은 문장형 (10~40자 권장).
- 해당 분류에 들어갈 내용이 없으면 빈 배열 [].
- JSON 외 다른 텍스트, 마크다운 코드펜스, 설명, 메타정보는 절대 출력하지 말 것.
`

// callGemini writes the body text to a file (debugDir if set, otherwise a
// tempfile) and invokes gemini-cli with the structured-extraction prompt.
// Returns the trimmed stdout content.
func callGemini(ctx context.Context, bodyText string, postingID uint, debugDir string) (string, error) {
	bodyDir, bodyName, cleanup, err := stageBody(bodyText, postingID, debugDir)
	if err != nil {
		return "", err
	}
	if cleanup != "" {
		defer func() { _ = os.Remove(cleanup) }()
	}

	cmdCtx, cancel := context.WithTimeout(ctx, geminiTimeout)
	defer cancel()

	cmd := exec.CommandContext(cmdCtx,
		"gemini",
		"--skip-trust",
		"-m", Model,
		"-p", promptTemplate+"\n\n@"+bodyName,
		"-o", "text",
	)
	cmd.Dir = bodyDir

	var stdout strings.Builder
	var stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	runErr := cmd.Run()
	if errors.Is(cmdCtx.Err(), context.DeadlineExceeded) {
		return "", fmt.Errorf("gemini timeout posting_id=%d after=%s", postingID, geminiTimeout)
	}
	if runErr != nil {
		return "", classifyGeminiError(runErr, stderr.String(), postingID)
	}

	return strings.TrimSpace(stdout.String()), nil
}

func stageBody(bodyText string, postingID uint, debugDir string) (string, string, string, error) {
	if debugDir != "" {
		name := fmt.Sprintf("%d_body.txt", postingID)
		path := filepath.Join(debugDir, name)
		if err := os.WriteFile(path, []byte(bodyText), 0o600); err != nil {
			return "", "", "", fmt.Errorf("write debug body: %w", err)
		}
		return debugDir, name, "", nil
	}

	f, err := os.CreateTemp("", "extract-*.txt")
	if err != nil {
		return "", "", "", fmt.Errorf("create tempfile: %w", err)
	}
	path := f.Name()
	if _, err := f.Write([]byte(bodyText)); err != nil {
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

func classifyGeminiError(err error, stderrText string, postingID uint) error {
	var execErr *exec.Error
	if errors.As(err, &execErr) && errors.Is(execErr.Err, exec.ErrNotFound) {
		return errors.New("gemini not found — install gemini-cli and ensure it is on PATH")
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return fmt.Errorf("gemini exit=%d posting_id=%d stderr=%q", exitErr.ExitCode(), postingID, tailLines(stderrText, 3))
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
