package mailing

import (
	"context"
	"strings"
	"testing"
	"time"

	"skipjd/internal/matcher"
	"skipjd/internal/model"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildDigestBodyIncludesAllFields(t *testing.T) {
	body := BuildDigestBody([]model.JobPosting{
		{
			Title:       "Backend Engineer",
			Company:     "Krafton",
			ClosingDate: "채용 시 마감",
			URL:         "https://jobs.example.com/postings/123",
		},
	})

	assert.Contains(t, body, "제목: Backend Engineer")
	assert.Contains(t, body, "회사: Krafton")
	assert.Contains(t, body, "마감일: 채용 시 마감")
	assert.Contains(t, body, "링크: https://jobs.example.com/postings/123")
}

func TestSMTPMailerSendDigestBuildsExpectedMessage(t *testing.T) {
	mailer := NewSMTPMailer(SMTPConfig{
		Host: "smtp.example.com",
		Port: 587,
		User: "smtp-user",
		Pass: "smtp-pass",
		From: "from@example.com",
		To:   "to@example.com",
	})

	var gotConfig SMTPConfig
	var gotMsg string
	mailer.sendMail = func(ctx context.Context, config SMTPConfig, to string, msg []byte) error {
		_ = to
		_ = ctx
		gotConfig = config
		gotMsg = string(msg)
		return nil
	}

	runAt := time.Date(2026, 3, 31, 9, 30, 0, 0, time.UTC)
	err := mailer.SendDigest(context.Background(), runAt, []model.JobPosting{
		{
			Title:       "AI Engineer",
			Company:     "Krafton",
			ClosingDate: "2026-04-01",
			URL:         "https://jobs.example.com/postings/456",
		},
	})
	require.NoError(t, err)

	assert.Equal(t, "smtp.example.com", gotConfig.Host)
	assert.Equal(t, 587, gotConfig.Port)
	assert.Equal(t, "smtp-user", gotConfig.User)
	assert.Equal(t, "smtp-pass", gotConfig.Pass)
	assert.Equal(t, "from@example.com", gotConfig.From)
	assert.Equal(t, "to@example.com", gotConfig.To)
	assert.Contains(t, gotMsg, "Subject: SkipJD Digest 2026-03-31T09:30:00Z (1 new postings)")
	assert.True(t, strings.Contains(gotMsg, "제목: AI Engineer"))
}

func TestSMTPMailerSendDigestSkipsWhenNoPostings(t *testing.T) {
	mailer := NewSMTPMailer(SMTPConfig{
		Host: "smtp.example.com",
		Port: 587,
		User: "smtp-user",
		Pass: "smtp-pass",
		From: "from@example.com",
		To:   "to@example.com",
	})

	called := false
	mailer.sendMail = func(ctx context.Context, config SMTPConfig, to string, msg []byte) error {
		_ = to
		_ = ctx
		_ = config
		_ = msg
		called = true
		return nil
	}

	err := mailer.SendDigest(context.Background(), time.Now(), nil)
	require.NoError(t, err)
	assert.False(t, called)
}

func TestSMTPMailerSendMatchDigestUsesDynamicRecipient(t *testing.T) {
	mailer := NewSMTPMailer(SMTPConfig{
		Host: "smtp.example.com",
		Port: 587,
		User: "smtp-user",
		Pass: "smtp-pass",
		From: "from@example.com",
		To:   "fallback@example.com",
	})

	var gotTo string
	var gotMsg string
	mailer.sendMail = func(ctx context.Context, config SMTPConfig, to string, msg []byte) error {
		_ = ctx
		_ = config
		gotTo = to
		gotMsg = string(msg)
		return nil
	}

	scored := []ScoredPosting{
		{
			Posting: model.JobPosting{
				Title:       "Game Server Engineer",
				Company:     "Krafton",
				ClosingDate: "2026-06-30",
				URL:         "https://jobs.example.com/postings/789",
			},
			Score: matcher.Score{
				Experience: matcher.CategoryScore{Hits: 2, Matched: []string{"Unity 3년", "라이브 서비스"}},
				Competency: matcher.CategoryScore{Hits: 1, Matched: []string{"C# 능숙"}},
				Total:      3,
			},
		},
	}
	sections := []Section{
		{Title: "추천 (점수순)", Items: scored},
	}
	runAt := time.Date(2026, 5, 14, 9, 0, 0, 0, time.UTC)
	err := mailer.SendMatchDigest(context.Background(), "user@example.com", runAt, sections)
	require.NoError(t, err)

	assert.Equal(t, "user@example.com", gotTo, "Rcpt must use dynamic to, not fallback config.To")
	assert.Contains(t, gotMsg, "To: user@example.com")
	assert.Contains(t, gotMsg, "From: from@example.com")
	assert.Contains(t, gotMsg, "Subject: SkipJD 추천 공고 2026-05-14 (1건)")
	assert.Contains(t, gotMsg, "── 추천 (점수순) ──")
	assert.Contains(t, gotMsg, "[1] 점수 3 (경험:2 / 역량:1 / 성향:0)")
	assert.Contains(t, gotMsg, "회사: Krafton")
	assert.Contains(t, gotMsg, "  · 경험: Unity 3년, 라이브 서비스")
}

func TestSMTPMailerSendMatchDigestRendersTwoSections(t *testing.T) {
	mailer := NewSMTPMailer(SMTPConfig{Host: "h", Port: 587, User: "u", Pass: "p", From: "f@x.com", To: "t@x.com"})
	var gotMsg string
	mailer.sendMail = func(ctx context.Context, config SMTPConfig, to string, msg []byte) error {
		_ = ctx
		_ = config
		_ = to
		gotMsg = string(msg)
		return nil
	}

	must := []ScoredPosting{
		{
			Posting: model.JobPosting{Title: "Must Posting", Company: "TargetCo", URL: "https://x/1"},
			Score:   matcher.Score{Total: 0},
		},
	}
	rec := []ScoredPosting{
		{
			Posting: model.JobPosting{Title: "Top Score", Company: "OtherCo", URL: "https://x/2"},
			Score:   matcher.Score{Total: 7, Experience: matcher.CategoryScore{Hits: 7, Matched: []string{"foo"}}},
		},
	}
	sections := []Section{
		{Title: "필수 표시 (회사·직무·경력 매칭)", Items: must},
		{Title: "추천 (점수순)", Items: rec},
	}
	err := mailer.SendMatchDigest(context.Background(), "user@example.com", time.Now(), sections)
	require.NoError(t, err)

	assert.Contains(t, gotMsg, "── 필수 표시 (회사·직무·경력 매칭) ──")
	assert.Contains(t, gotMsg, "── 추천 (점수순) ──")
	assert.Contains(t, gotMsg, "회사: TargetCo")
	assert.Contains(t, gotMsg, "회사: OtherCo")
	assert.Contains(t, gotMsg, "(2건)", "subject reports total across sections")
}

func TestSMTPMailerSendMatchDigestSkipsWhenEmpty(t *testing.T) {
	mailer := NewSMTPMailer(SMTPConfig{Host: "h", Port: 587, User: "u", Pass: "p", From: "f@x.com", To: "t@x.com"})
	called := false
	mailer.sendMail = func(ctx context.Context, config SMTPConfig, to string, msg []byte) error {
		_ = ctx
		_ = config
		_ = to
		_ = msg
		called = true
		return nil
	}
	err := mailer.SendMatchDigest(context.Background(), "user@example.com", time.Now(), nil)
	require.NoError(t, err)
	assert.False(t, called)

	// Also: sections present but all items empty → still skip.
	err = mailer.SendMatchDigest(context.Background(), "user@example.com", time.Now(), []Section{
		{Title: "empty section", Items: nil},
	})
	require.NoError(t, err)
	assert.False(t, called)
}

func TestSMTPMailerSendDigestPropagatesContextCancellationDuringSend(t *testing.T) {
	mailer := NewSMTPMailer(SMTPConfig{
		Host: "smtp.example.com",
		Port: 587,
		User: "smtp-user",
		Pass: "smtp-pass",
		From: "from@example.com",
		To:   "to@example.com",
	})

	started := make(chan struct{})
	mailer.sendMail = func(ctx context.Context, config SMTPConfig, to string, msg []byte) error {
		_ = to
		_ = config
		_ = msg
		close(started)
		<-ctx.Done()
		return ctx.Err()
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- mailer.SendDigest(ctx, time.Now(), []model.JobPosting{
			{
				Title:       "AI Engineer",
				Company:     "Krafton",
				ClosingDate: "2026-04-01",
				URL:         "https://jobs.example.com/postings/456",
			},
		})
	}()

	<-started
	cancel()

	err := <-done
	require.ErrorIs(t, err, context.Canceled)
}
