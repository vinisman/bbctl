package config

import (
	"log/slog"
	"os"
	"strconv"
)

// Config holds global CLI configuration
type Config struct {
	BaseURL          string
	Token            string
	Username         string
	Password         string
	PageSize         int
	GlobalMaxWorkers int
	Insecure         bool
}

var (
	GlobalCfg        *Config
	GlobalLogger     *slog.Logger
	GlobalMaxWorkers int

	// HTTP client retry parameters (BITBUCKET_RETRY_*)
	GlobalRetryMaxAttempts int
	GlobalRetryBaseDelayMs int
)

// LoadConfig loads configuration from environment variables
func LoadConfig() (*Config, error) {
	pageSize := 50
	if val := os.Getenv("BITBUCKET_PAGE_SIZE"); val != "" {
		if ps, err := strconv.Atoi(val); err == nil {
			pageSize = ps
		}
	}

	maxWorkers := 5
	if val := os.Getenv("BITBUCKET_MAX_WORKERS"); val != "" {
		if mw, err := strconv.Atoi(val); err == nil {
			maxWorkers = mw
		}
	}
	GlobalMaxWorkers = maxWorkers

	// HTTP client retry: total attempts (including the first) and base backoff delay
	GlobalRetryMaxAttempts = 4
	if val := os.Getenv("BITBUCKET_RETRY_MAX_ATTEMPTS"); val != "" {
		if ra, err := strconv.Atoi(val); err == nil && ra >= 1 {
			GlobalRetryMaxAttempts = ra
		}
	}

	GlobalRetryBaseDelayMs = 500
	if val := os.Getenv("BITBUCKET_RETRY_BASE_DELAY_MS"); val != "" {
		if rd, err := strconv.Atoi(val); err == nil && rd >= 0 {
			GlobalRetryBaseDelayMs = rd
		}
	}

	baseURL := os.Getenv("BITBUCKET_BASE_URL")
	token := os.Getenv("BITBUCKET_TOKEN")
	username := os.Getenv("BITBUCKET_USERNAME")
	password := os.Getenv("BITBUCKET_PASSWORD")
	insecure := os.Getenv("BITBUCKET_INSECURE") == "true"

	cfg := &Config{
		BaseURL:          baseURL,
		Token:            token,
		Username:         username,
		Password:         password,
		PageSize:         pageSize,
		GlobalMaxWorkers: maxWorkers,
		Insecure:         insecure,
	}
	GlobalCfg = cfg

	return cfg, nil
}
