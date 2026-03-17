package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

type Config struct {
	AppEnv               string
	Port                 string
	DBUser               string
	DBPass               string
	DBHost               string
	DBPort               string
	DBName               string
	DBAutoMigrate        bool
	RequireDBTLS         bool
	JWTSecret            string
	JWTExpire            int
	SignInRateLimit      int
	SignInRateWindowSecs int
}

func Load() Config {
	return Config{
		AppEnv:        strings.ToLower(getEnvOrDefault("APP_ENV", "development")),
		Port:          getEnv("PORT"),
		DBUser:        getEnv("DB_USER"),
		DBPass:        getEnv("DB_PASS"),
		DBHost:        getEnv("DB_HOST"),
		DBPort:        getEnv("DB_PORT"),
		DBName:        getEnv("DB_NAME"),
		DBAutoMigrate: getParseEnv("DB_AUTO_MIGRATE", strconv.ParseBool),
		RequireDBTLS:  getParseEnvOrDefault("REQUIRE_DB_TLS", false, strconv.ParseBool),

		JWTSecret:            getEnv("JWT_SECRET"),
		JWTExpire:            getParseEnv("JWT_EXPIRE", strconv.Atoi),
		SignInRateLimit:      getParseEnvOrDefault("SIGNIN_RATE_LIMIT", 5, strconv.Atoi),
		SignInRateWindowSecs: getParseEnvOrDefault("SIGNIN_RATE_WINDOW_SECS", 60, strconv.Atoi),
	}
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

func getEnvOrDefault(key, defaultValue string) string {
	v := os.Getenv(key)
	if v == "" {
		return defaultValue
	}

	return v
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
