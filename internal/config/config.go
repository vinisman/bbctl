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
}

var (
	GlobalCfg        *Config
	GlobalLogger     *slog.Logger
	GlobalMaxWorkers int
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

	baseURL := os.Getenv("BITBUCKET_BASE_URL")
	token := os.Getenv("BITBUCKET_TOKEN")
	username := os.Getenv("BITBUCKET_USERNAME")
	password := os.Getenv("BITBUCKET_PASSWORD")

	cfg := &Config{
		BaseURL:          baseURL,
		Token:            token,
		Username:         username,
		Password:         password,
		PageSize:         pageSize,
		GlobalMaxWorkers: maxWorkers,
	}
	GlobalCfg = cfg

	return cfg, nil
}
