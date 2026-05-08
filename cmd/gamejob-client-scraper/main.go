package main

import (
	"context"
	"encoding/json"
	"flag"
	"log"
	"os"
	"strings"
	"time"

	"skipjd/internal/gamejob"
)

func main() {
	todayDateFlag := flag.String("today-date", "", "Reference date in YYYY-MM-DD (Seoul); defaults to current Seoul date")
	maxPagesFlag := flag.Int("max-pages", gamejob.DefaultMaxPages, "Maximum number of listing pages to scan")
	flag.Parse()

	loc, err := time.LoadLocation("Asia/Seoul")
	if err != nil {
		log.Fatalf("failed to load Asia/Seoul location: %v", err)
	}

	var todayDate time.Time
	if todayDateValue := strings.TrimSpace(*todayDateFlag); todayDateValue != "" {
		todayDate, err = time.ParseInLocation("2006-01-02", todayDateValue, loc)
		if err != nil {
			log.Fatalf("invalid --today-date: %v", err)
		}
	} else {
		now := time.Now().In(loc)
		todayDate = time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, loc)
	}

	scraper, err := gamejob.NewClientScraper()
	if err != nil {
		log.Fatalf("failed to initialize scraper: %v", err)
	}
	postings, err := scraper.Scrape(context.Background(), gamejob.ScrapeOptions{
		TodayDate: todayDate,
		MaxPages:  *maxPagesFlag,
	})
	if err != nil {
		log.Fatalf("failed to scrape postings: %v", err)
	}

	encoder := json.NewEncoder(os.Stdout)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(newOutput(postings, loc)); err != nil {
		log.Fatalf("failed to encode output: %v", err)
	}
}

type output struct {
	Postings []outputPosting `json:"postings"`
}

type outputPosting struct {
	SourceKey          string `json:"source_key"`
	Title              string `json:"title"`
	Company            string `json:"company"`
	DutyCodes          []int  `json:"duty_codes,omitempty"`
	URL                string `json:"url"`
	ClosingDate        string `json:"closing_date"`
	MinExperienceYears int    `json:"min_experience_years"`
	ObservedDate       string `json:"observed_date"`
}

func newOutput(postings []gamejob.ScrapedPosting, loc *time.Location) output {
	items := make([]outputPosting, 0, len(postings))
	itemIndexBySourceKey := make(map[string]int, len(postings))
	for _, posting := range postings {
		if index, exists := itemIndexBySourceKey[posting.SourceKey]; exists {
			items[index].DutyCodes = gamejob.NormalizeDutyCodes(append(items[index].DutyCodes, posting.DutyCode))
			continue
		}

		items = append(items, outputPosting{
			SourceKey:          posting.SourceKey,
			Title:              posting.Title,
			Company:            posting.Company,
			DutyCodes:          []int{posting.DutyCode},
			URL:                posting.URL,
			ClosingDate:        posting.ClosingDate,
			MinExperienceYears: posting.MinExperienceYears,
			ObservedDate:       posting.ObservedDate.In(loc).Format("2006-01-02"),
		})
		itemIndexBySourceKey[posting.SourceKey] = len(items) - 1
	}

	return output{Postings: items}
}
