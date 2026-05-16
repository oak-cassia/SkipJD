package crawler

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
	"time"

	"skipjd/internal/gamejob"
	"skipjd/internal/model"
)

type Output struct {
	Postings []Posting `json:"postings"`
}

type Posting struct {
	Title              string `json:"title"`
	Company            string `json:"company"`
	DutyCodes          []int  `json:"duty_codes,omitempty"`
	URL                string `json:"url"`
	ClosingDate        string `json:"closing_date"`
	MinExperienceYears *int   `json:"min_experience_years"`
}

func Encode(postings []model.JobPosting, dutyCodesBySourceKey map[string][]int) (string, error) {
	output := Output{
		Postings: make([]Posting, 0, len(postings)),
	}

	for _, posting := range postings {
		var dutyCodes []int
		if src := dutyCodesBySourceKey[posting.SourceKey]; len(src) > 0 {
			dutyCodes = make([]int, len(src))
			copy(dutyCodes, src)
		}

		var minExp *int
		if posting.MinExperienceYears != nil {
			v := *posting.MinExperienceYears
			minExp = &v
		}

		output.Postings = append(output.Postings, Posting{
			Title:              posting.Title,
			Company:            posting.Company,
			DutyCodes:          dutyCodes,
			URL:                posting.URL,
			ClosingDate:        posting.ClosingDate,
			MinExperienceYears: minExp,
		})
	}

	var buf bytes.Buffer
	encoder := json.NewEncoder(&buf)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(output); err != nil {
		return "", err
	}

	return buf.String(), nil
}

func Parse(outputText string, seenAt time.Time) ([]model.JobPosting, map[string][]int, error) {
	var output Output
	if err := json.Unmarshal([]byte(outputText), &output); err != nil {
		return nil, nil, err
	}

	postings := make([]model.JobPosting, 0, len(output.Postings))
	dutyCodesBySourceKey := make(map[string][]int, len(output.Postings))
	for i, posting := range output.Postings {
		title := strings.TrimSpace(posting.Title)
		company := strings.TrimSpace(posting.Company)
		postingURL := strings.TrimSpace(posting.URL)
		closingDate := strings.TrimSpace(posting.ClosingDate)

		if title == "" {
			return nil, nil, fmt.Errorf("posting %d: title is required", i)
		}
		if company == "" {
			return nil, nil, fmt.Errorf("posting %d: company is required", i)
		}
		if postingURL == "" {
			return nil, nil, fmt.Errorf("posting %d: url is required", i)
		}
		if closingDate == "" {
			return nil, nil, fmt.Errorf("posting %d: closing_date is required", i)
		}
		if err := validatePosting(title, postingURL); err != nil {
			return nil, nil, fmt.Errorf("posting %d: %w", i, err)
		}

		sourceKey, err := buildSourceKey(postingURL)
		if err != nil {
			return nil, nil, fmt.Errorf("posting %d: %w", i, err)
		}

		var minExperienceYears *int
		if posting.MinExperienceYears != nil {
			if *posting.MinExperienceYears < 0 {
				return nil, nil, fmt.Errorf("posting %d: min_experience_years must be non-negative", i)
			}
			minExperienceYears = new(int)
			*minExperienceYears = *posting.MinExperienceYears
		}
		var normalizedDutyCodes []int
		if len(posting.DutyCodes) > 0 {
			normalizedDutyCodes = make([]int, 0, len(posting.DutyCodes))
			for _, value := range posting.DutyCodes {
				if value <= 0 {
					return nil, nil, fmt.Errorf("posting %d: duty_codes must contain positive integers", i)
				}
				normalizedDutyCodes = append(normalizedDutyCodes, value)
			}
			normalizedDutyCodes = gamejob.NormalizeDutyCodes(normalizedDutyCodes)
		}
		if len(normalizedDutyCodes) > 0 {
			dutyCodesBySourceKey[sourceKey] = normalizedDutyCodes
		}

		postings = append(postings, model.JobPosting{
			Source:             gamejob.SourceName,
			SourceKey:          sourceKey,
			Title:              title,
			Company:            company,
			URL:                postingURL,
			ClosingDate:        closingDate,
			MinExperienceYears: minExperienceYears,
			FirstSeenAt:        seenAt,
			LastSeenAt:         seenAt,
		})
	}

	return postings, dutyCodesBySourceKey, nil
}

func validatePosting(title, rawURL string) error {
	parsedURL, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil {
		return fmt.Errorf("url must be a valid absolute URL: %w", err)
	}
	if parsedURL.Scheme != "http" && parsedURL.Scheme != "https" {
		return fmt.Errorf("url must use http or https")
	}
	if parsedURL.Host == "" {
		return fmt.Errorf("url must be absolute")
	}

	if strings.HasSuffix(strings.ToLower(parsedURL.Hostname()), "gamejob.co.kr") {
		if !isGameJobDetailURL(parsedURL) {
			return fmt.Errorf("url must point to a GameJob detail page")
		}
		if isTruncatedGameJobTitle(title) {
			return fmt.Errorf("title appears truncated; likely from a GameJob promo section")
		}
	}

	return nil
}

func isGameJobDetailURL(parsedURL *url.URL) bool {
	if parsedURL == nil {
		return false
	}

	if parsedURL.Path == "/Recruit/GI_Read/View" {
		giNo := strings.TrimSpace(parsedURL.Query().Get("GI_No"))
		if giNo == "" {
			return false
		}
		for _, r := range giNo {
			if r < '0' || r > '9' {
				return false
			}
		}
		return true
	}

	return strings.HasPrefix(parsedURL.Path, "/Recruit/GI_Read/")
}

func isTruncatedGameJobTitle(title string) bool {
	trimmed := strings.TrimSpace(title)
	return strings.HasSuffix(trimmed, "...") || strings.HasSuffix(trimmed, "…")
}

func buildSourceKey(rawURL string) (string, error) {
	trimmed := strings.TrimSpace(rawURL)
	if trimmed == "" {
		return "", fmt.Errorf("source key requires a non-empty url")
	}

	parsedURL, err := url.Parse(trimmed)
	if err != nil {
		return trimmed, nil
	}

	parsedURL.Fragment = ""
	normalized := parsedURL.String()
	if normalized == "" {
		return trimmed, nil
	}

	return normalized, nil
}
