package utils

import (
	"log/slog"
	"os"

	"github.com/go-playground/validator/v10"
	"github.com/joho/godotenv"
	"github.com/vinisman/bbctl/models"

	"gopkg.in/yaml.v3"
)

type Config struct {
	BitbucketURL   string
	BitbucketToken string
}

// Global vars
var (
	ProjectKey     string
	GlobalURL      string
	GlobalToken    string
	EnvFile        string
	InputFile      string
	BuildKey       string
	Branch         string
	OutputFile     string
	FieldsToShow   string
	ManifestFile   string
	ManifestExists bool
	TemplateStr    string
	FilterExpr     string
	Debug          bool
)

func LoadConfig() Config {
	if EnvFile != "" {
		if err := godotenv.Load(EnvFile); err != nil {
			slog.Info("Failed to load .env file", slog.String("file", EnvFile))
		}
	} else {
		_ = godotenv.Load()
	}

	// Priority: flags > env > .env
	url := firstNonEmpty(GlobalURL, os.Getenv("BITBUCKET_URL"))
	token := firstNonEmpty(GlobalToken, os.Getenv("BITBUCKET_TOKEN"))

	if url == "" || token == "" {
		slog.Info("BITBUCKET_URL and BITBUCKET_TOKEN are required.")
		os.Exit(1)
	}

	return Config{
		BitbucketURL:   url,
		BitbucketToken: token,
	}
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}

func ReadRepositoryYaml(file string) []models.RepoEntity {
	f, err := os.Open(file)
	if err != nil {
		slog.Debug("An error occurred", slog.String("error", err.Error()))
		slog.Info("failed to open file", slog.String("file", file))
		return nil
	}
	defer f.Close()
	var out models.RepoListInput
	yaml.NewDecoder(f).Decode(&out)
	return out.Repositories
}

func ReadRepositoryWithBuildsYaml(file string) []models.RepoWithBuildKeys {
	data, err := os.ReadFile(file)
	if err != nil {
		slog.Error("Failed to open file", slog.String("file", file), slog.Any("err", err))
		os.Exit(1)
	}

	var out models.RepoBuildFile
	if err := yaml.Unmarshal(data, &out); err != nil {
		slog.Error("Failed to decode YAML", slog.String("file", file), slog.Any("err", err))
		os.Exit(1)
	}

	v := validator.New()
	if err := v.Struct(out); err != nil {
		slog.Error("Validation error in YAML", slog.String("file", file), slog.Any("err", err))
		os.Exit(1)
	}

	return out.Repositories
}
