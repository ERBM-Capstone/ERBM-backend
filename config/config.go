package config

import (
	"log"
	"os"
	"strconv"
	"time"

	"github.com/joho/godotenv"
)

// Config holds all application configuration loaded from environment variables.
type Config struct {
	Port                       string
	DatabaseURL                string
	JWTSecret                  string
	JWTIssuer                  string
	JWTAccessTokenTTL          time.Duration
	JWTRefreshSecret           string
	JWTRefreshTokenTTL         time.Duration
	RefreshCookieSecure        bool
	AuthLoginLockDuration      time.Duration
	AuthMaxFailedLoginAttempts int
}

// Load reads .env (if present) and returns a populated Config.
func Load() *Config {
	if err := godotenv.Load(); err != nil {
		log.Println("No .env file found, reading from environment")
	}

	return &Config{
		Port:                       getEnv("PORT", "8080"),
		DatabaseURL:                getEnv("DATABASE_URL", ""),
		JWTSecret:                  getEnv("JWT_SECRET", ""),
		JWTIssuer:                  getEnv("JWT_ISSUER", "erbm"),
		JWTAccessTokenTTL:          getDurationEnv("JWT_ACCESS_TOKEN_TTL", 15*time.Minute),
		JWTRefreshSecret:           getEnv("JWT_REFRESH_SECRET", ""),
		JWTRefreshTokenTTL:         getDurationEnv("JWT_REFRESH_TTL", 168*time.Hour),
		RefreshCookieSecure:        getBoolEnv("REFRESH_COOKIE_SECURE", false),
		AuthLoginLockDuration:      getDurationEnv("AUTH_LOGIN_LOCK_DURATION", 3*time.Minute),
		AuthMaxFailedLoginAttempts: getIntEnv("AUTH_MAX_FAILED_LOGIN_ATTEMPTS", 10),
	}
}

func getDurationEnv(key string, fallback time.Duration) time.Duration {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}

	duration, err := time.ParseDuration(value)
	if err != nil {
		log.Printf("invalid duration for %s, using default", key)
		return fallback
	}

	return duration
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func getBoolEnv(key string, fallback bool) bool {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}

	parsed, err := strconv.ParseBool(value)
	if err != nil {
		log.Printf("invalid boolean for %s, using default", key)
		return fallback
	}

	return parsed
}
func getIntEnv(key string, fallback int) int {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}

	parsed, err := strconv.Atoi(value)
	if err != nil || parsed < 1 {
		log.Printf("invalid integer for %s, using default", key)
		return fallback
	}
	return parsed
}
