package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"skipjd/internal/gamejob"
)

func main() {
	companiesFlag := flag.String("companies", "", "Comma-separated exact company names")
	lastUpdatedFlag := flag.String("last-updated", "", "Inclusive cutoff date in YYYY-MM-DD (Seoul)")
	todayDateFlag := flag.String("today-date", "", "Reference date in YYYY-MM-DD (Seoul); defaults to current Seoul date")
	maxPagesFlag := flag.Int("max-pages", gamejob.DefaultMaxPages, "Maximum number of listing pages to scan")
	flag.Parse()

	loc, err := time.LoadLocation("Asia/Seoul")
	if err != nil {
		log.Fatalf("failed to load Asia/Seoul location: %v", err)
	}

	todayDateValue := strings.TrimSpace(*todayDateFlag)
	if todayDateValue == "" {
		todayDateValue = time.Now().In(loc).Format("2006-01-02")
	}

	lastUpdated, err := parseDate(*lastUpdatedFlag, loc)
	if err != nil {
		log.Fatalf("invalid --last-updated: %v", err)
	}
	todayDate, err := parseDate(todayDateValue, loc)
	if err != nil {
		log.Fatalf("invalid --today-date: %v", err)
	}

	companies := parseCompanies(*companiesFlag)
	if len(companies) == 0 {
		log.Fatalf("--companies is required")
	}

	scraper := gamejob.NewClientScraper(nil)
	postings, err := scraper.Collect(context.Background(), gamejob.CollectOptions{
		PreferredCompanies: companies,
		LastUpdated:        lastUpdated,
		TodayDate:          todayDate,
		MaxPages:           *maxPagesFlag,
	})
	if err != nil {
		log.Fatalf("failed to collect postings: %v", err)
	}

	encoder := json.NewEncoder(os.Stdout)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(gamejob.NewOutput(postings)); err != nil {
		log.Fatalf("failed to encode output: %v", err)
	}
}

func parseDate(value string, loc *time.Location) (time.Time, error) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return time.Time{}, fmt.Errorf("value is required")
	}
	return time.ParseInLocation("2006-01-02", trimmed, loc)
}

func parseCompanies(value string) []string {
	parts := strings.Split(value, ",")
	companies := make([]string, 0, len(parts))
	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed == "" {
			continue
		}
		companies = append(companies, trimmed)
	}
	return companies
}
