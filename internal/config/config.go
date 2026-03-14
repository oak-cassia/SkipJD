package config

import (
	"fmt"
	"os"
	"strconv"
)

type Config struct {
	Port          string
	DBUser        string
	DBPass        string
	DBHost        string
	DBPort        string
	DBName        string
	DBAutoMigrate bool
}

func Load() Config {
	return Config{
		Port:          getEnv("PORT"),
		DBUser:        getEnv("DB_USER"),
		DBPass:        getEnv("DB_PASS"),
		DBHost:        getEnv("DB_HOST"),
		DBPort:        getEnv("DB_PORT"),
		DBName:        getEnv("DB_NAME"),
		DBAutoMigrate: getParseEnv("DB_AUTO_MIGRATE", strconv.ParseBool),
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
