package userextractor

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/ledongthuc/pdf"
)

// ReadResume reads the resume / self-introduction file at path and returns
// its plain-text contents. Supported extensions:
//   - .txt, .md  → raw bytes (UTF-8 assumed)
//   - .pdf       → pdf.GetPlainText() (text PDFs only; image-only PDFs return
//     empty or near-empty text and the caller should error out)
func ReadResume(path string) (string, error) {
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".txt", ".md":
		b, err := os.ReadFile(path)
		if err != nil {
			return "", fmt.Errorf("read %s: %w", path, err)
		}
		return string(b), nil
	case ".pdf":
		return readPDF(path)
	default:
		return "", fmt.Errorf("unsupported resume extension %q (allowed: .txt, .md, .pdf)", ext)
	}
}

// ReadResumes reads each path via ReadResume and concatenates the results
// into a single text block. Each section is prefixed with a `[SOURCE: path]`
// marker and separated by a horizontal rule so the LLM can tell the
// documents apart. Returns an error if any single file fails to parse.
func ReadResumes(paths []string) (string, error) {
	if len(paths) == 0 {
		return "", errors.New("no input files")
	}
	var sb strings.Builder
	for i, p := range paths {
		text, err := ReadResume(p)
		if err != nil {
			return "", err
		}
		if i > 0 {
			sb.WriteString("\n\n========================================\n\n")
		}
		fmt.Fprintf(&sb, "[SOURCE: %s]\n\n", p)
		sb.WriteString(text)
	}
	return sb.String(), nil
}

func readPDF(path string) (string, error) {
	f, r, err := pdf.Open(path)
	if err != nil {
		return "", fmt.Errorf("open pdf %s: %w", path, err)
	}
	defer f.Close()

	reader, err := r.GetPlainText()
	if err != nil {
		return "", fmt.Errorf("extract pdf text: %w", err)
	}

	var buf bytes.Buffer
	if _, err := buf.ReadFrom(reader); err != nil {
		return "", fmt.Errorf("read pdf text: %w", err)
	}
	return buf.String(), nil
}

