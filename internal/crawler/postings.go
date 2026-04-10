package crawler

import (
	"time"

	"skipjd/internal/gamejob"
	"skipjd/internal/model"
)

func (c *Crawler) toJobPostings(
	scrapedPostings []gamejob.ScrapedPosting,
	seenAt time.Time,
) []model.JobPosting {
	postings := make([]model.JobPosting, 0, len(scrapedPostings))
	seenSourceKeys := make(map[string]struct{}, len(scrapedPostings))

	for _, scrapedPosting := range scrapedPostings {
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

func intPtr(value int) *int {
	copied := value
	return &copied
}
