package geminiexec

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
)

// fenceRE strips the optional leading "```" / "```json" and trailing "```"
// that gemini-cli sometimes wraps the JSON response in despite the prompt.
var fenceRE = regexp.MustCompile("(?m)^```(?:json)?\\s*|\\s*```$")

// Result is the parsed shape of a successful gemini extraction response.
// experience / competency / trait are the three classification buckets used
// by both JD and user-side extraction.
type Result struct {
	Experience []string
	Competency []string
	Trait      []string
}

// ErrInvalidShape is returned when the model's response is not valid JSON
// or does not contain the expected keys / value types.
var ErrInvalidShape = errors.New("invalid response shape")

// ParseResponse parses a gemini extraction response into experience /
// competency / trait string slices.
func ParseResponse(raw string) (*Result, error) {
	if raw == "" {
		return nil, ErrInvalidShape
	}
	cleaned := strings.TrimSpace(fenceRE.ReplaceAllString(raw, ""))

	var data map[string]any
	if err := json.Unmarshal([]byte(cleaned), &data); err != nil {
		return nil, ErrInvalidShape
	}

	experience, err := extractStringList(data, "experience")
	if err != nil {
		return nil, err
	}
	competency, err := extractStringList(data, "competency")
	if err != nil {
		return nil, err
	}
	trait, err := extractStringList(data, "trait")
	if err != nil {
		return nil, err
	}

	return &Result{
		Experience: experience,
		Competency: competency,
		Trait:      trait,
	}, nil
}

// EncodeArray emits a JSON array with UTF-8 preserved as-is and without
// escaping HTML-unsafe ASCII (<, >, &), so the stored extraction reads the
// same as the source body.
func EncodeArray(items []string) (string, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(items); err != nil {
		return "", fmt.Errorf("marshal: %w", err)
	}
	return strings.TrimRight(buf.String(), "\n"), nil
}

func extractStringList(data map[string]any, key string) ([]string, error) {
	v, ok := data[key]
	if !ok {
		return []string{}, nil
	}
	list, ok := v.([]any)
	if !ok {
		return nil, ErrInvalidShape
	}
	items := make([]string, 0, len(list))
	for _, item := range list {
		s := strings.TrimSpace(coerceString(item))
		if s != "" {
			items = append(items, s)
		}
	}
	return items, nil
}

func coerceString(v any) string {
	switch s := v.(type) {
	case string:
		return s
	case nil:
		return ""
	default:
		return fmt.Sprint(v)
	}
}
