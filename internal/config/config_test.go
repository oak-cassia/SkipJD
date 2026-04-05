package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadReadsMailConfig(t *testing.T) {
	setDatabaseEnv(t)
	setSMTPEnv(t)

	cfg := LoadSMTPConfig()

	assert.Equal(t, "smtp.example.com", cfg.SMTPHost)
	assert.Equal(t, 587, cfg.SMTPPort)
	assert.Equal(t, "smtp-user", cfg.SMTPUser)
	assert.Equal(t, "smtp-pass", cfg.SMTPPass)
	assert.Equal(t, "from@example.com", cfg.MailFrom)
	assert.Equal(t, "to@example.com", cfg.MailTo)
}

func TestLoadDatabaseConfigDoesNotRequireSMTPEnv(t *testing.T) {
	setDatabaseEnv(t)

	cfg := LoadDatabaseConfig()

	assert.Equal(t, "root", cfg.DBUser)
	assert.Equal(t, "password", cfg.DBPass)
	assert.Equal(t, "127.0.0.1", cfg.DBHost)
	assert.Equal(t, "3306", cfg.DBPort)
	assert.Equal(t, "skipjd", cfg.DBName)
	assert.False(t, cfg.RequireDBTLS)
}

func TestLoadPanicsWhenSMTPPortIsInvalid(t *testing.T) {
	setDatabaseEnv(t)
	setSMTPEnv(t)
	t.Setenv("SMTP_PORT", "not-a-number")

	require.Panics(t, func() {
		_ = LoadSMTPConfig()
	})
}

func TestLoadPanicsWhenSMTPPortIsNot587(t *testing.T) {
	setDatabaseEnv(t)
	setSMTPEnv(t)
	t.Setenv("SMTP_PORT", "465")

	require.PanicsWithValue(t, "invalid env SMTP_PORT=\"465\": only 587 is supported", func() {
		_ = LoadSMTPConfig()
	})
}

func TestLoadPanicsWhenMailToHasMultipleAddresses(t *testing.T) {
	setDatabaseEnv(t)
	setSMTPEnv(t)
	t.Setenv("MAIL_TO", "first@example.com,second@example.com")

	require.Panics(t, func() {
		_ = LoadSMTPConfig()
	})
}

func TestLoadPanicsWhenSMTPHostIsMissing(t *testing.T) {
	setDatabaseEnv(t)
	setSMTPEnv(t)
	t.Setenv("SMTP_HOST", "")

	require.PanicsWithValue(t, "missing required env: SMTP_HOST", func() {
		_ = LoadSMTPConfig()
	})
}

func setDatabaseEnv(t *testing.T) {
	t.Helper()

	t.Setenv("DB_USER", "root")
	t.Setenv("DB_PASS", "password")
	t.Setenv("DB_HOST", "127.0.0.1")
	t.Setenv("DB_PORT", "3306")
	t.Setenv("DB_NAME", "skipjd")
	t.Setenv("REQUIRE_DB_TLS", "false")
}

func setSMTPEnv(t *testing.T) {
	t.Helper()

	t.Setenv("SMTP_HOST", "smtp.example.com")
	t.Setenv("SMTP_PORT", "587")
	t.Setenv("SMTP_USER", "smtp-user")
	t.Setenv("SMTP_PASS", "smtp-pass")
	t.Setenv("MAIL_FROM", "from@example.com")
	t.Setenv("MAIL_TO", "to@example.com")
}
