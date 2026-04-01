package config

import (
	"fmt"
	"net/mail"
	"os"
	"strconv"
	"strings"
)

type Config struct {
	DBUser       string
	DBPass       string
	DBHost       string
	DBPort       string
	DBName       string
	RequireDBTLS bool
	SMTPHost     string
	SMTPPort     int
	SMTPUser     string
	SMTPPass     string
	MailFrom     string
	MailTo       string
}

const requiredSMTPPort = 587

func Load() Config {
	return Config{
		DBUser:       getEnv("DB_USER"),
		DBPass:       getEnv("DB_PASS"),
		DBHost:       getEnv("DB_HOST"),
		DBPort:       getEnv("DB_PORT"),
		DBName:       getEnv("DB_NAME"),
		RequireDBTLS: getParseEnvOrDefault("REQUIRE_DB_TLS", false, strconv.ParseBool),
		SMTPHost:     getEnv("SMTP_HOST"),
		SMTPPort:     getSMTPPort(),
		SMTPUser:     getEnv("SMTP_USER"),
		SMTPPass:     getEnv("SMTP_PASS"),
		MailFrom:     getSingleAddressEnv("MAIL_FROM"),
		MailTo:       getSingleAddressEnv("MAIL_TO"),
	}
}

func getSMTPPort() int {
	port := getParseEnv("SMTP_PORT", strconv.Atoi)
	if port != requiredSMTPPort {
		panic(fmt.Sprintf("invalid env SMTP_PORT=%q: only %d is supported", strconv.Itoa(port), requiredSMTPPort))
	}
	return port
}

func getEnv(key string) string {
	v := os.Getenv(key)
	if v == "" {
		panic(fmt.Sprintf("missing required env: %s", key))
	}
	return v
}

func getParseEnv[T any](key string, parse func(string) (T, error)) T {
	v := getEnv(key)

	parsed, err := parse(v)
	if err != nil {
		panic(fmt.Sprintf("invalid env %s=%q: %v", key, v, err))
	}

	return parsed
}

func getParseEnvOrDefault[T any](key string, defaultValue T, parse func(string) (T, error)) T {
	v := os.Getenv(key)
	if v == "" {
		return defaultValue
	}

	parsed, err := parse(v)
	if err != nil {
		panic(fmt.Sprintf("invalid env %s=%q: %v", key, v, err))
	}

	return parsed
}

func getSingleAddressEnv(key string) string {
	v := strings.TrimSpace(getEnv(key))
	if strings.Contains(v, ",") {
		panic(fmt.Sprintf("invalid env %s=%q: only one email address is allowed", key, v))
	}

	addr, err := mail.ParseAddress(v)
	if err != nil {
		panic(fmt.Sprintf("invalid env %s=%q: %v", key, v, err))
	}
	if addr.Address != v {
		panic(fmt.Sprintf("invalid env %s=%q: plain email address required", key, v))
	}

	return v
}
