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

	"skipjd/internal/cmdutil"
	"skipjd/internal/gamejob"
	"skipjd/internal/repository"
)

// seedConfig bundles the flag values for run.
type seedConfig struct {
	userID       uint
	email        string
	dutyCodesCSV string
	companyCount int
	companiesCSV string
	careerYears  int
	apply        bool
}

func main() {
	userID := flag.Uint("user-id", 1, "target user id")
	emailFlag := flag.String("email", "dummy@example.com", "target user email")
	dutyCodesFlag := flag.String("duty-codes", "1,3", "comma-separated duty codes ("+gamejob.DutyCodesHelp()+")")
	companyCount := flag.Int("company-count", 5, "number of top-N companies from job_postings.company DISTINCT")
	companiesFlag := flag.String("companies", "", "comma-separated company names (overrides --company-count)")
	careerYearsFlag := flag.Int("career-years", -1, "user experience years (>=0; -1 means unset/clear)")
	apply := flag.Bool("apply", false, "apply changes (default: dry-run)")
	flag.Parse()

	cfg := seedConfig{
		userID:       *userID,
		email:        *emailFlag,
		dutyCodesCSV: *dutyCodesFlag,
		companyCount: *companyCount,
		companiesCSV: *companiesFlag,
		careerYears:  *careerYearsFlag,
		apply:        *apply,
	}
	if err := run(cfg); err != nil {
		log.Fatal(err)
	}
}

// run is split from main so log.Fatal cannot skip the deferred cancel.
func run(cfg seedConfig) error {
	ctx, cancel := cmdutil.SetupContext(0)
	defer cancel()

	db := cmdutil.MustConnectDB()

	dutyCodes, err := parseCSVInts(cfg.dutyCodesCSV)
	if err != nil {
		return fmt.Errorf("invalid --duty-codes: %w", err)
	}

	repo := repository.NewPreferencesRepository(db)

	companies, err := resolveCompanies(ctx, repo, cfg.companiesCSV, cfg.companyCount)
	if err != nil {
		return fmt.Errorf("resolve companies: %w", err)
	}

	careerYears := resolveCareerYears(cfg.careerYears)

	fmt.Printf("planned preferences for user_id=%d:\n", cfg.userID)
	fmt.Printf("  email      : %s\n", cfg.email)
	fmt.Printf("  duty_codes : %v\n", dutyCodes)
	fmt.Printf("  companies  : %v\n", companies)
	fmt.Printf("  career_yrs : %s\n", formatCareerYears(careerYears))

	if !cfg.apply {
		fmt.Println("\n(dry-run) re-run with --apply to persist.")
		return nil
	}

	user, err := repo.EnsureUser(ctx, cfg.userID, cfg.email)
	if err != nil {
		return fmt.Errorf("ensure user: %w", err)
	}
	fmt.Printf("\nuser_id=%d ready (created_at=%s)\n", user.ID, user.CreatedAt.Format("2006-01-02 15:04:05"))

	if err = repo.ReplaceUserDutyPreferences(ctx, cfg.userID, dutyCodes); err != nil {
		return fmt.Errorf("replace duty preferences: %w", err)
	}
	if err = repo.ReplaceUserCompanyPreferences(ctx, cfg.userID, companies); err != nil {
		return fmt.Errorf("replace company preferences: %w", err)
	}
	if err = repo.ReplaceUserCareer(ctx, cfg.userID, careerYears); err != nil {
		return fmt.Errorf("replace user career: %w", err)
	}

	finalDuties, err := repo.GetUserDutyCodes(ctx, cfg.userID)
	if err != nil {
		return fmt.Errorf("read back duty preferences: %w", err)
	}
	finalCompanies, err := repo.GetUserCompanyNames(ctx, cfg.userID)
	if err != nil {
		return fmt.Errorf("read back company preferences: %w", err)
	}
	finalCareer, err := repo.GetUserCareer(ctx, cfg.userID)
	if err != nil {
		return fmt.Errorf("read back user career: %w", err)
	}

	fmt.Printf("\napplied. current state for user_id=%d:\n", cfg.userID)
	fmt.Printf("  duty_codes : %v\n", finalDuties)
	fmt.Printf("  companies  : %v\n", finalCompanies)
	fmt.Printf("  career_yrs : %s\n", formatCareerYears(finalCareer))
	return nil
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
