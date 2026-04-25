// seed-preferences is a one-off seeder that writes duty/company preferences
// for a given user. Mimics cmd/normalize-companies in style (dry-run + --apply).
//
// Usage:
//
//	go run ./cmd/seed-preferences                           # dry-run (defaults)
//	go run ./cmd/seed-preferences --apply                   # seed user_id=1 with duty [1,3] + top 5 companies
//	go run ./cmd/seed-preferences --user-id 2 --duty-codes 1,16 --company-count 10 --apply
//	go run ./cmd/seed-preferences --companies "넵튠,크래프톤" --apply
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"strconv"
	"strings"

	"github.com/joho/godotenv"

	"skipjd/internal/config"
	"skipjd/internal/database"
	"skipjd/internal/repository"
)

func main() {
	userID := flag.Uint("user-id", 1, "target user id")
	emailFlag := flag.String("email", "dummy@example.com", "target user email")
	dutyCodesFlag := flag.String("duty-codes", "1,3", "comma-separated duty codes (1=client, 3=server, 16=AI)")
	companyCount := flag.Int("company-count", 5, "number of top-N companies from job_postings.company DISTINCT")
	companiesFlag := flag.String("companies", "", "comma-separated company names (overrides --company-count)")
	careerYearsFlag := flag.Int("career-years", -1, "user experience years (>=0; -1 means unset/clear)")
	apply := flag.Bool("apply", false, "apply changes (default: dry-run)")
	flag.Parse()

	_ = godotenv.Load()

	ctx := context.Background()

	db, err := database.NewGormDB(config.LoadDatabaseConfig())
	if err != nil {
		log.Fatalf("failed to connect db: %v", err)
	}

	dutyCodes, err := parseCSVInts(*dutyCodesFlag)
	if err != nil {
		log.Fatalf("invalid --duty-codes: %v", err)
	}

	repo := repository.NewPreferencesRepository(db)

	companies, err := resolveCompanies(ctx, repo, *companiesFlag, *companyCount)
	if err != nil {
		log.Fatalf("resolve companies: %v", err)
	}

	careerYears := resolveCareerYears(*careerYearsFlag)

	fmt.Printf("planned preferences for user_id=%d:\n", *userID)
	fmt.Printf("  email      : %s\n", *emailFlag)
	fmt.Printf("  duty_codes : %v\n", dutyCodes)
	fmt.Printf("  companies  : %v\n", companies)
	fmt.Printf("  career_yrs : %s\n", formatCareerYears(careerYears))

	if !*apply {
		fmt.Println("\n(dry-run) re-run with --apply to persist.")
		return
	}

	user, err := repo.EnsureUser(ctx, uint(*userID), *emailFlag)
	if err != nil {
		log.Fatalf("ensure user: %v", err)
	}
	fmt.Printf("\nuser_id=%d ready (created_at=%s)\n", user.ID, user.CreatedAt.Format("2006-01-02 15:04:05"))

	if err := repo.ReplaceUserDutyPreferences(ctx, uint(*userID), dutyCodes); err != nil {
		log.Fatalf("replace duty preferences: %v", err)
	}
	if err := repo.ReplaceUserCompanyPreferences(ctx, uint(*userID), companies); err != nil {
		log.Fatalf("replace company preferences: %v", err)
	}
	if err := repo.ReplaceUserCareer(ctx, uint(*userID), careerYears); err != nil {
		log.Fatalf("replace user career: %v", err)
	}

	finalDuties, err := repo.GetUserDutyCodes(ctx, uint(*userID))
	if err != nil {
		log.Fatalf("read back duty preferences: %v", err)
	}
	finalCompanies, err := repo.GetUserCompanyNames(ctx, uint(*userID))
	if err != nil {
		log.Fatalf("read back company preferences: %v", err)
	}
	finalCareer, err := repo.GetUserCareer(ctx, uint(*userID))
	if err != nil {
		log.Fatalf("read back user career: %v", err)
	}

	fmt.Printf("\napplied. current state for user_id=%d:\n", *userID)
	fmt.Printf("  duty_codes : %v\n", finalDuties)
	fmt.Printf("  companies  : %v\n", finalCompanies)
	fmt.Printf("  career_yrs : %s\n", formatCareerYears(finalCareer))
}

func resolveCareerYears(raw int) *int {
	if raw < 0 {
		return nil
	}
	v := raw
	return &v
}

func formatCareerYears(years *int) string {
	if years == nil {
		return "(unset)"
	}
	return strconv.Itoa(*years)
}

func resolveCompanies(ctx context.Context, repo *repository.PreferencesRepository, raw string, topN int) ([]string, error) {
	if strings.TrimSpace(raw) != "" {
		parts := strings.Split(raw, ",")
		out := make([]string, 0, len(parts))
		for _, p := range parts {
			if trimmed := strings.TrimSpace(p); trimmed != "" {
				out = append(out, trimmed)
			}
		}
		return out, nil
	}

	candidates, err := repo.ListDistinctCompanyNames(ctx)
	if err != nil {
		return nil, err
	}
	if topN <= 0 || topN >= len(candidates) {
		return candidates, nil
	}
	return candidates[:topN], nil
}

func parseCSVInts(s string) ([]int, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil, nil
	}

	parts := strings.Split(s, ",")
	out := make([]int, 0, len(parts))
	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed == "" {
			continue
		}
		v, err := strconv.Atoi(trimmed)
		if err != nil {
			return nil, fmt.Errorf("parse %q: %w", trimmed, err)
		}
		out = append(out, v)
	}
	return out, nil
}
