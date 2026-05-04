package extractor

import (
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
type Result struct {
	Experience []string
	Competency []string
	Trait      []string
}

var errInvalidShape = errors.New("invalid response shape")

func parseResponse(raw string) (*Result, error) {
	if raw == "" {
		return nil, errInvalidShape
	}
	cleaned := strings.TrimSpace(fenceRE.ReplaceAllString(raw, ""))

	var data map[string]any
	if err := json.Unmarshal([]byte(cleaned), &data); err != nil {
		return nil, errInvalidShape
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

func extractStringList(data map[string]any, key string) ([]string, error) {
	v, ok := data[key]
	if !ok {
		return []string{}, nil
	}
	list, ok := v.([]any)
	if !ok {
		return nil, errInvalidShape
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
