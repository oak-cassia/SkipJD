package mailing

import (
	"context"
	"strings"
	"testing"
	"time"

	"skipjd/internal/model"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildDigestBodyIncludesAllFields(t *testing.T) {
	body := buildDigestBody([]model.JobPosting{
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
	mailer.sendMail = func(ctx context.Context, config SMTPConfig, msg []byte) error {
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
	mailer.sendMail = func(ctx context.Context, config SMTPConfig, msg []byte) error {
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
	mailer.sendMail = func(ctx context.Context, config SMTPConfig, msg []byte) error {
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
