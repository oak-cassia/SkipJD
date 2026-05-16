// notify scores candidate job postings against each user's extraction and
// sends a top-N digest email per user. Pipeline (per user):
//
//  1. Load user + UserExtraction + UserDutyPreference rows.
//  2. Fetch candidate JobPostings (filtered by duty codes if any are set,
//     else all postings from --source).
//  3. Drop any posting already recorded in user_notification_logs for this
//     user — "send once, never resend" semantics.
//  4. Fetch each remaining candidate's JobPostingExtraction.
//  5. Score each candidate against the user via internal/matcher.
//  6. Sort by total score desc, keep top --top-n, send via SMTPMailer to
//     the user's email (or print to stdout with --dry-run).
//  7. On successful send, record (user_id, job_posting_id) in
//     user_notification_logs so the same postings are skipped next run.
//     Dry-run does NOT record.
//
// By default the run iterates over every user that has a UserExtraction
// row. Pass --user-id <N> to target a single user (useful for testing).
//
// Reads DB config from project root .env (DB_USER, DB_PASS, DB_HOST,
// DB_PORT, DB_NAME, REQUIRE_DB_TLS). For non-dry-run sends, also requires
// SMTP_HOST, SMTP_PORT=587, SMTP_USER, SMTP_PASS, MAIL_FROM, MAIL_TO.
//
// Usage:
//
//	go run ./cmd/notify --dry-run                  # preview every user
//	go run ./cmd/notify --user-id 1 --dry-run      # preview just user 1
//	go run ./cmd/notify --top-n 10                 # send to every user
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log"
	"sort"
	"time"

	"gorm.io/gorm"

	"skipjd/internal/cmdutil"
	"skipjd/internal/config"
	"skipjd/internal/gamejob"
	"skipjd/internal/mailing"
	"skipjd/internal/matcher"
	"skipjd/internal/model"
	"skipjd/internal/repository"
)

type userStatus int

const (
	statusSent userStatus = iota
	statusEmpty
)

func main() {
	userID := flag.Uint("user-id", 0, "target user id (0 = iterate all users with extractions)")
	source := flag.String("source", gamejob.SourceName, "JD source key to draw candidates from")
	topN := flag.Int("top-n", 10, "max postings to include per digest")
	dryRun := flag.Bool("dry-run", false, "print the digest body to stdout instead of sending mail")
	deadline := flag.Duration("deadline", 0, "max duration for the whole run (0 = no deadline)")
	flag.Parse()

	if *topN <= 0 {
		log.Fatalf("--top-n must be > 0")
	}

	ctx, cancel := cmdutil.SetupContext(*deadline)
	defer cancel()

	db := cmdutil.MustConnectDB()
	if err := db.AutoMigrate(
		&model.User{},
		&model.UserCareer{},
		&model.UserDutyPreference{},
		&model.UserCompanyPreference{},
		&model.UserNotificationLog{},
	); err != nil {
		log.Fatalf("migrate: %v", err)
	}

	crawlerRepo := repository.NewCrawlerRepository(db)
	userExtRepo := repository.NewUserExtractionRepository(db)
	prefsRepo := repository.NewPreferencesRepository(db)
	notifyLogRepo := repository.NewUserNotificationLogRepository(db)

	var mailer *mailing.SMTPMailer
	if !*dryRun {
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

	var targets []uint
	if *userID != 0 {
		targets = []uint{*userID}
	} else {
		var err error
		targets, err = userExtRepo.ListUserIDsWithExtraction(ctx)
		if err != nil {
			log.Fatalf("resolve targets: %v", err)
		}
	}
	if len(targets) == 0 {
		log.Println("no target users (run cmd/user-extractor first)")
		return
	}
	log.Printf("targets=%d dry_run=%v top_n=%d source=%s", len(targets), *dryRun, *topN, *source)

	var ok, errs, empty int
	for _, uid := range targets {
		status, err := notifyUser(ctx, db, crawlerRepo, userExtRepo, prefsRepo, notifyLogRepo, mailer, *source, *topN, *dryRun, uid)
		switch {
		case err != nil:
			errs++
			log.Printf("[user_id=%d] FAIL %v", uid, err)
		case status == statusEmpty:
			empty++
		default:
			ok++
		}
	}
	log.Printf("done sent=%d empty=%d failed=%d", ok, empty, errs)
	if errs > 0 {
		// Non-zero exit so cron/launchd can surface batch failures.
		log.Fatalf("%d user(s) failed", errs)
	}
}

func notifyUser(
	ctx context.Context,
	db *gorm.DB,
	crawlerRepo *repository.CrawlerRepository,
	userExtRepo *repository.UserExtractionRepository,
	prefsRepo *repository.PreferencesRepository,
	notifyLogRepo *repository.UserNotificationLogRepository,
	mailer *mailing.SMTPMailer,
	source string,
	topN int,
	dryRun bool,
	userID uint,
) (userStatus, error) {
	user, err := getUser(ctx, db, userID)
	if err != nil {
		return 0, fmt.Errorf("load user: %w", err)
	}

	userExt, err := userExtRepo.GetByUserID(ctx, userID)
	if err != nil {
		return 0, fmt.Errorf("load user extraction: %w", err)
	}
	if userExt == nil {
		return 0, fmt.Errorf("no extraction yet — run cmd/user-extractor first")
	}

	userTriple, err := decodeTriple(userExt.Experience, userExt.Competency, userExt.Trait)
	if err != nil {
		return 0, fmt.Errorf("decode user triple: %w", err)
	}

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

	sent, err := notifyLogRepo.GetSentPostingIDs(ctx, userID)
	if err != nil {
		return 0, fmt.Errorf("load sent log: %w", err)
	}
	fresh := make([]model.JobPosting, 0, len(candidates))
	for _, p := range candidates {
		if _, was := sent[p.ID]; was {
			continue
		}
		fresh = append(fresh, p)
	}

	postingIDs := make([]uint, 0, len(fresh))
	for _, p := range fresh {
		postingIDs = append(postingIDs, p.ID)
	}
	extractions, err := crawlerRepo.GetExtractionsByPostingIDs(ctx, postingIDs)
	if err != nil {
		return 0, fmt.Errorf("load extractions: %w", err)
	}

	scoredAll := make([]mailing.ScoredPosting, 0, len(fresh))
	for _, p := range fresh {
		var score matcher.Score
		if ex, ok := extractions[p.ID]; ok {
			jdTriple, derr := decodeTriple(ex.Experience, ex.Competency, ex.Trait)
			if derr != nil {
				log.Printf("[user_id=%d] decode jd posting_id=%d: %v", userID, p.ID, derr)
			} else {
				score = matcher.Match(userTriple, jdTriple)
			}
		}
		scoredAll = append(scoredAll, mailing.ScoredPosting{Posting: p, Score: score})
	}

	mustShow := computeMustShow(scoredAll, companyNames, dutyCodes, careerYears)
	sort.SliceStable(mustShow, func(i, j int) bool {
		return mustShow[i].Score.Total > mustShow[j].Score.Total
	})
	mustShowIDs := make(map[uint]struct{}, len(mustShow))
	for _, m := range mustShow {
		mustShowIDs[m.Posting.ID] = struct{}{}
	}

	recommended := make([]mailing.ScoredPosting, 0, len(scoredAll))
	for _, s := range scoredAll {
		if _, inMust := mustShowIDs[s.Posting.ID]; inMust {
			continue
		}
		if s.Score.Total == 0 {
			continue
		}
		recommended = append(recommended, s)
	}
	sort.SliceStable(recommended, func(i, j int) bool {
		return recommended[i].Score.Total > recommended[j].Score.Total
	})
	if len(recommended) > topN {
		recommended = recommended[:topN]
	}

	sections := []mailing.Section{
		{Title: "필수 표시 (회사·직무·경력 매칭)", Items: mustShow},
		{Title: "추천 (점수순)", Items: recommended},
	}
	total := mailing.TotalItems(sections)
	log.Printf("[user_id=%d] candidates=%d already_sent=%d fresh=%d must_show=%d recommended=%d",
		userID, len(candidates), len(candidates)-len(fresh), len(fresh), len(mustShow), len(recommended))

	if total == 0 {
		return statusEmpty, nil
	}

	if dryRun {
		fmt.Printf("\n--- dry-run: user_id=%d to=%s total=%d ---\n", userID, user.Email, total)
		fmt.Print(mailing.BuildMatchDigestBody(sections))
		return statusSent, nil
	}

	sentAt := time.Now()
	if err := mailer.SendMatchDigest(ctx, user.Email, sentAt, sections); err != nil {
		return 0, fmt.Errorf("send mail: %w", err)
	}

	sentIDs := make([]uint, 0, total)
	for _, sec := range sections {
		for _, item := range sec.Items {
			sentIDs = append(sentIDs, item.Posting.ID)
		}
	}
	if err := notifyLogRepo.Record(ctx, userID, sentIDs, sentAt); err != nil {
		// Mail was already sent — log loudly but keep batch going. Worst
		// case the same postings reappear next run.
		log.Printf("[user_id=%d] WARN record sent_log failed (mail was sent): %v", userID, err)
	}
	log.Printf("[user_id=%d] sent to=%s total=%d (must_show=%d recommended=%d)",
		userID, user.Email, total, len(mustShow), len(recommended))
	return statusSent, nil
}

// computeMustShow returns postings that satisfy ALL of:
//
//   - the user has both duty and company preferences set (otherwise the rule
//     is undefined and the must-show set is empty);
//   - the posting's company matches one in the user's company preference
//     list (NormalizeCompanyName applied on both sides);
//   - the posting's MinExperienceYears is nil OR userCareer is nil OR
//     MinExperienceYears <= userCareer + 3 (most lenient interpretation —
//     unknown experience never excludes).
//
// Duty overlap is already enforced upstream by ListJobPostingsByDutyCodes
// when dutyCodes is non-empty; we still gate on `len(dutyCodes) > 0` here
// to keep the rule contract explicit.
func computeMustShow(
	scored []mailing.ScoredPosting,
	companyNames []string,
	dutyCodes []int,
	careerYears *int,
) []mailing.ScoredPosting {
	if len(dutyCodes) == 0 || len(companyNames) == 0 {
		return nil
	}
	userCompanies := make(map[string]struct{}, len(companyNames))
	for _, c := range companyNames {
		userCompanies[gamejob.NormalizeCompanyName(c)] = struct{}{}
	}

	out := make([]mailing.ScoredPosting, 0)
	for _, s := range scored {
		company := gamejob.NormalizeCompanyName(s.Posting.Company)
		if _, ok := userCompanies[company]; !ok {
			continue
		}
		// experience check: nil on either side = no constraint (lenient).
		if s.Posting.MinExperienceYears != nil && careerYears != nil &&
			*s.Posting.MinExperienceYears > *careerYears+3 {
			continue
		}
		out = append(out, s)
	}
	return out
}

func getUser(ctx context.Context, db *gorm.DB, userID uint) (*model.User, error) {
	var user model.User
	err := db.WithContext(ctx).First(&user, userID).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, fmt.Errorf("user_id=%d not found", userID)
	}
	if err != nil {
		return nil, err
	}
	return &user, nil
}

func decodeTriple(experience, competency, trait string) (matcher.Triple, error) {
	var exp, comp, tr []string
	if experience != "" {
		if err := json.Unmarshal([]byte(experience), &exp); err != nil {
			return matcher.Triple{}, fmt.Errorf("experience: %w", err)
		}
	}
	if competency != "" {
		if err := json.Unmarshal([]byte(competency), &comp); err != nil {
			return matcher.Triple{}, fmt.Errorf("competency: %w", err)
		}
	}
	if trait != "" {
		if err := json.Unmarshal([]byte(trait), &tr); err != nil {
			return matcher.Triple{}, fmt.Errorf("trait: %w", err)
		}
	}
	return matcher.Triple{Experience: exp, Competency: comp, Trait: tr}, nil
}
