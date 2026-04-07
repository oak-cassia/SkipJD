package crawler

import (
	"strings"
	"time"

	"skipjd/internal/gamejob"
	"skipjd/internal/model"
)

func (c *Crawler) toJobPostings(
	scrapedPostings []gamejob.ScrapedPosting,
	preferredCompanies []string,
	seenAt time.Time,
) []model.JobPosting {
	companySet := normalizeCompanies(preferredCompanies)
	postings := make([]model.JobPosting, 0, len(scrapedPostings))
	seenSourceKeys := make(map[string]struct{}, len(scrapedPostings))

	for _, scrapedPosting := range scrapedPostings {
		if _, ok := companySet[canonicalCompanyName(scrapedPosting.Company)]; !ok {
			continue
		}
		if _, exists := seenSourceKeys[scrapedPosting.SourceKey]; exists {
			continue
		}

		postings = append(postings, model.JobPosting{
			Source:             appName,
			SourceKey:          scrapedPosting.SourceKey,
			Title:              scrapedPosting.Title,
			Company:            scrapedPosting.Company,
			URL:                scrapedPosting.URL,
			ClosingDate:        scrapedPosting.ClosingDate,
			MinExperienceYears: intPtr(scrapedPosting.MinExperienceYears),
			FirstSeenAt:        seenAt,
			LastSeenAt:         seenAt,
		})
		seenSourceKeys[scrapedPosting.SourceKey] = struct{}{}
	}

	return postings
}

func normalizeCompanies(companies []string) map[string]struct{} {
	normalized := make(map[string]struct{}, len(companies))
	for _, company := range companies {
		trimmed := canonicalCompanyName(company)
		if trimmed == "" {
			continue
		}
		normalized[trimmed] = struct{}{}
	}
	return normalized
}

func canonicalCompanyName(value string) string {
	normalized := strings.Join(strings.Fields(strings.TrimSpace(value)), " ")
	if normalized == "" {
		return ""
	}

	prefixes := []string{"㈜", "(주)", "（주）", "주식회사"}
	suffixes := []string{"㈜", "(주)", "（주）", "주식회사"}

	for {
		changed := false
		for _, prefix := range prefixes {
			if strings.HasPrefix(normalized, prefix) {
				normalized = strings.Join(strings.Fields(strings.TrimSpace(strings.TrimPrefix(normalized, prefix))), " ")
				changed = true
			}
		}
		for _, suffix := range suffixes {
			if strings.HasSuffix(normalized, suffix) {
				normalized = strings.Join(strings.Fields(strings.TrimSpace(strings.TrimSuffix(normalized, suffix))), " ")
				changed = true
			}
		}
		if !changed {
			break
		}
	}

	return normalized
}

func intPtr(value int) *int {
	copied := value
	return &copied
}
