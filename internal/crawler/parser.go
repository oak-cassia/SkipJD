package crawler

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
	"time"

	"skipjd/internal/model"
)

type collectedPostingsOutput struct {
	Postings []collectedPosting `json:"postings"`
}

type collectedPosting struct {
	Title              string `json:"title"`
	Company            string `json:"company"`
	URL                string `json:"url"`
	ClosingDate        string `json:"closing_date"`
	MinExperienceYears *int   `json:"min_experience_years"`
}

func parseCollectedPostings(outputText string, seenAt time.Time) ([]model.JobPosting, error) {
	var output collectedPostingsOutput
	if err := json.Unmarshal([]byte(outputText), &output); err != nil {
		return nil, err
	}

	postings := make([]model.JobPosting, 0, len(output.Postings))
	for i, posting := range output.Postings {
		title := strings.TrimSpace(posting.Title)
		company := strings.TrimSpace(posting.Company)
		postingURL := strings.TrimSpace(posting.URL)
		closingDate := strings.TrimSpace(posting.ClosingDate)

		if title == "" {
			return nil, fmt.Errorf("posting %d: title is required", i)
		}
		if company == "" {
			return nil, fmt.Errorf("posting %d: company is required", i)
		}
		if postingURL == "" {
			return nil, fmt.Errorf("posting %d: url is required", i)
		}
		if closingDate == "" {
			return nil, fmt.Errorf("posting %d: closing_date is required", i)
		}

		sourceKey, err := buildSourceKey(postingURL)
		if err != nil {
			return nil, fmt.Errorf("posting %d: %w", i, err)
		}

		var minExperienceYears *int
		if posting.MinExperienceYears != nil {
			if *posting.MinExperienceYears < 0 {
				return nil, fmt.Errorf("posting %d: min_experience_years must be non-negative", i)
			}
			minExperienceYears = new(*posting.MinExperienceYears)
		}

		postings = append(postings, model.JobPosting{
			Source:             appName,
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

	return postings, nil
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
