// recommend produces a purely rule-based job digest per user: candidate
// postings filtered by the user's duty codes, then narrowed to preferred
// companies within the user's experience range (see internal/recommend).
//
// No LLM extraction is involved — neither the JD-side extractor nor the
// user-side user-extractor is required, so this runs even when the
// gemini-backed stages have never executed. It is the rule-based-only
// counterpart to cmd/notify, which additionally ranks by LLM match score.
//
// Reads DB config from project root .env (DB_USER, DB_PASS, DB_HOST, DB_PORT,
// DB_NAME). With --send it also requires SMTP_HOST, SMTP_PORT, SMTP_USER,
// SMTP_PASS, MAIL_FROM, MAIL_TO and delivers to MAIL_TO.
//
// Usage:
//
//	go run ./cmd/recommend                    # dry-run, every user with prefs
//	go run ./cmd/recommend --user-id 1         # dry-run, just user 1
//	go run ./cmd/recommend --user-id 1 --send   # email the digest (to MAIL_TO)
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"time"

	"skipjd/internal/cmdutil"
	"skipjd/internal/config"
	"skipjd/internal/gamejob"
	"skipjd/internal/mailing"
	"skipjd/internal/model"
	"skipjd/internal/recommend"
	"skipjd/internal/repository"
)

func main() {
	userID := flag.Uint("user-id", 0, "target user id (0 = every user with preferences)")
	source := flag.String("source", gamejob.SourceName, "JD source key to draw candidates from")
	send := flag.Bool("send", false, "send the digest by email (default: print to stdout)")
	deadline := flag.Duration("deadline", 0, "max duration for the whole run (0 = no deadline)")
	flag.Parse()

	// Non-zero exit so cron/launchd can surface batch failures.
	if err := run(*deadline, *source, *send, *userID); err != nil {
		log.Fatal(err)
	}
}

// run is split from main so log.Fatal cannot skip the deferred cancel.
func run(deadline time.Duration, source string, send bool, userID uint) error {
	ctx, cancel := cmdutil.SetupContext(deadline)
	defer cancel()

	db := cmdutil.MustConnectDB()
	if err := db.AutoMigrate(
		&model.User{},
		&model.UserDutyPreference{},
		&model.UserCompanyPreference{},
	); err != nil {
		return fmt.Errorf("migrate: %w", err)
	}

	crawlerRepo := repository.NewCrawlerRepository(db)
	prefsRepo := repository.NewPreferencesRepository(db)

	var mailer *mailing.SMTPMailer
	if send {
		smtp := config.LoadSMTPConfig()
		mailer = mailing.NewSMTPMailer(mailing.SMTPConfig{
			Host: smtp.SMTPHost,
			Port: smtp.SMTPPort,
			User: smtp.SMTPUser,
			Pass: smtp.SMTPPass,
			From: smtp.MailFrom,
			To:   smtp.MailTo,
		})
	}

	targets, err := resolveTargets(ctx, prefsRepo, userID)
	if err != nil {
		return fmt.Errorf("resolve targets: %w", err)
	}
	if len(targets) == 0 {
		log.Println("no target users (seed preferences with cmd/seed-preferences first)")
		return nil
	}
	log.Printf("targets=%d send=%v source=%s", len(targets), send, source)

	var ok, empty, errs int
	for _, uid := range targets {
		n, err := recommendUser(ctx, crawlerRepo, prefsRepo, mailer, source, uid)
		switch {
		case err != nil:
			errs++
			log.Printf("[user_id=%d] FAIL %v", uid, err)
		case n == 0:
			empty++
		default:
			ok++
		}
	}
	log.Printf("done sent=%d empty=%d failed=%d", ok, empty, errs)
	if errs > 0 {
		return fmt.Errorf("%d user(s) failed", errs)
	}
	return nil
}

// recommendUser computes the rule-based digest for one user and either prints
// it (nil mailer = dry-run) or emails it. Returns the number of postings in
// the digest.
func recommendUser(
	ctx context.Context,
	crawlerRepo *repository.CrawlerRepository,
	prefsRepo *repository.PreferencesRepository,
	mailer *mailing.SMTPMailer,
	source string,
	userID uint,
) (int, error) {
	dutyCodes, err := prefsRepo.GetUserDutyCodes(ctx, userID)
	if err != nil {
		return 0, fmt.Errorf("load duty codes: %w", err)
	}
	companyNames, err := prefsRepo.GetUserCompanyNames(ctx, userID)
	if err != nil {
		return 0, fmt.Errorf("load company prefs: %w", err)
	}
	careerYears, err := prefsRepo.GetUserCareer(ctx, userID)
	if err != nil {
		return 0, fmt.Errorf("load career: %w", err)
	}

	var candidates []model.JobPosting
	if len(dutyCodes) > 0 {
		candidates, err = crawlerRepo.ListJobPostingsByDutyCodes(ctx, source, dutyCodes)
	} else {
		candidates, err = crawlerRepo.ListJobPostingsBySource(ctx, source)
	}
	if err != nil {
		return 0, fmt.Errorf("load candidates: %w", err)
	}

	matched := recommend.Match(candidates, companyNames, careerYears)
	log.Printf("[user_id=%d] candidates=%d matched=%d", userID, len(candidates), len(matched))
	if len(matched) == 0 {
		return 0, nil
	}

	if mailer == nil {
		fmt.Printf("\n--- dry-run: user_id=%d (rule-based, %d postings) ---\n", userID, len(matched))
		fmt.Print(mailing.BuildDigestBody(matched))
		return len(matched), nil
	}

	if err := mailer.SendDigest(ctx, time.Now(), matched); err != nil {
		return 0, fmt.Errorf("send mail: %w", err)
	}
	log.Printf("[user_id=%d] sent total=%d", userID, len(matched))
	return len(matched), nil
}

// resolveTargets returns the explicit user id when set, otherwise every user
// id in the table (users without preferences simply yield an empty digest).
func resolveTargets(ctx context.Context, prefsRepo *repository.PreferencesRepository, userID uint) ([]uint, error) {
	if userID != 0 {
		return []uint{userID}, nil
	}
	return prefsRepo.ListUserIDs(ctx)
}
