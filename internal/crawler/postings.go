package crawler

import (
	"time"

	"skipjd/internal/gamejob"
	"skipjd/internal/model"
)

func (c *Crawler) toJobPostings(
	scrapedPostings []gamejob.ScrapedPosting,
	seenAt time.Time,
) ([]model.JobPosting, map[string][]int) {
	postings := make([]model.JobPosting, 0, len(scrapedPostings))
	postingIndexBySourceKey := make(map[string]int, len(scrapedPostings))
	dutyCodesBySourceKey := make(map[string][]int, len(scrapedPostings))

	for _, scrapedPosting := range scrapedPostings {
		dutyCodesBySourceKey[scrapedPosting.SourceKey] = gamejob.NormalizeDutyCodes(
			append(dutyCodesBySourceKey[scrapedPosting.SourceKey], scrapedPosting.DutyCode),
		)
		if index, exists := postingIndexBySourceKey[scrapedPosting.SourceKey]; exists {
			postings[index].LastSeenAt = seenAt
			continue
		}

		minExp := scrapedPosting.MinExperienceYears
		postings = append(postings, model.JobPosting{
			Source:             appName,
			SourceKey:          scrapedPosting.SourceKey,
			Title:              scrapedPosting.Title,
			Company:            scrapedPosting.Company,
			URL:                scrapedPosting.URL,
			ClosingDate:        scrapedPosting.ClosingDate,
			MinExperienceYears: &minExp,
			FirstSeenAt:        seenAt,
			LastSeenAt:         seenAt,
		})
		postingIndexBySourceKey[scrapedPosting.SourceKey] = len(postings) - 1
	}

	return postings, dutyCodesBySourceKey
}
