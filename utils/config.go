package utils

import (
	"fmt"
	"log/slog"
	"os"

	"github.com/joho/godotenv"
)

var (
	Logger      *slog.Logger
	ProjectKey  string
	AllProjects bool
	GlobalURL   string
	GlobalToken string
	EnvFile     string
	Debug       bool
	PageSize    int
	Cfg         *Config
)

type Config struct {
	BitbucketURL   string
	BitbucketToken string
	ProjectKey     string
	PageSize       int
}

func LoadConfig() (*Config, error) {
	if EnvFile != "" {
		if err := godotenv.Load(EnvFile); err != nil {
			slog.Info("Failed to load .env file", "file", EnvFile)
		}
	} else {
		_ = godotenv.Load()
	}

	url := firstNonEmpty(GlobalURL, os.Getenv("BITBUCKET_BASE_URL"), os.Getenv("BITBUCKET_URL"))
	token := firstNonEmpty(GlobalToken, os.Getenv("BITBUCKET_TOKEN"))
	project := firstNonEmpty(ProjectKey, os.Getenv("BITBUCKET_PROJECT_KEY"))

	pageSize := PageSize
	if pageSize == 0 {
		pageSize = 50
		if ps := os.Getenv("BITBUCKET_PAGE_SIZE"); ps != "" {
			_, err := fmt.Sscanf(ps, "%d", &pageSize)
			if err != nil {
				slog.Info("Invalid BITBUCKET_PAGE_SIZE env var, fallback to default 50")
				pageSize = 50
			}
		}
	}

	if url == "" {
		return nil, fmt.Errorf("parameter BITBUCKET_BASE_URL or --base-url must be set")
	}
	if token == "" {
		return nil, fmt.Errorf("parameter BITBUCKET_TOKEN or --token must be set")
	}

	return &Config{
		BitbucketURL:   url,
		BitbucketToken: token,
		ProjectKey:     project,
		PageSize:       pageSize,
	}, nil
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}
