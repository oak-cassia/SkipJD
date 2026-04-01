package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadReadsMailConfig(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("SMTP_PORT", "587")
	t.Setenv("MAIL_FROM", "from@example.com")
	t.Setenv("MAIL_TO", "to@example.com")

	cfg := Load()

	assert.Equal(t, "smtp.example.com", cfg.SMTPHost)
	assert.Equal(t, 587, cfg.SMTPPort)
	assert.Equal(t, "smtp-user", cfg.SMTPUser)
	assert.Equal(t, "smtp-pass", cfg.SMTPPass)
	assert.Equal(t, "from@example.com", cfg.MailFrom)
	assert.Equal(t, "to@example.com", cfg.MailTo)
}

func TestLoadPanicsWhenSMTPPortIsInvalid(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("SMTP_PORT", "not-a-number")

	require.Panics(t, func() {
		_ = Load()
	})
}

func TestLoadPanicsWhenSMTPPortIsNot587(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("SMTP_PORT", "465")

	require.PanicsWithValue(t, "invalid env SMTP_PORT=\"465\": only 587 is supported", func() {
		_ = Load()
	})
}

func TestLoadPanicsWhenMailToHasMultipleAddresses(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("MAIL_TO", "first@example.com,second@example.com")

	require.Panics(t, func() {
		_ = Load()
	})
}

func TestLoadPanicsWhenSMTPHostIsMissing(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("SMTP_HOST", "")

	require.PanicsWithValue(t, "missing required env: SMTP_HOST", func() {
		_ = Load()
	})
}

func setRequiredEnv(t *testing.T) {
	t.Helper()

	t.Setenv("DB_USER", "root")
	t.Setenv("DB_PASS", "password")
	t.Setenv("DB_HOST", "127.0.0.1")
	t.Setenv("DB_PORT", "3306")
	t.Setenv("DB_NAME", "skipjd")
	t.Setenv("REQUIRE_DB_TLS", "false")

	t.Setenv("SMTP_HOST", "smtp.example.com")
	t.Setenv("SMTP_PORT", "587")
	t.Setenv("SMTP_USER", "smtp-user")
	t.Setenv("SMTP_PASS", "smtp-pass")
	t.Setenv("MAIL_FROM", "from@example.com")
	t.Setenv("MAIL_TO", "to@example.com")
}
