package bitbucket

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/vinisman/bbctl/internal/config"
	openapi "github.com/vinisman/bitbucket-sdk-go/openapi"
)

type Client struct {
	api     *openapi.APIClient
	logger  *slog.Logger
	client  *http.Client
	authCtx context.Context
	config  *config.Config
	Logger  *slog.Logger
}

func NewClient(ctx context.Context) (*Client, error) {
	config.GlobalLogger.Debug("Initializing Bitbucket client")

	config.GlobalLogger.Debug("Preparing OpenAPI configuration")
	cfgOpenAPI := openapi.NewConfiguration()
	cfgOpenAPI.Servers = openapi.ServerConfigurations{{URL: config.GlobalCfg.BaseURL}}
	config.GlobalLogger.Debug("Server URL set to", "url", config.GlobalCfg.BaseURL)
	cfgOpenAPI.AddDefaultHeader("User-Agent", "bbctl/1.0")
	config.GlobalLogger.Debug("Added default header", "header", "User-Agent", "value", "bbctl/1.0")
	cfgOpenAPI.AddDefaultHeader("X-Content-Type-Options", "nosniff")
	config.GlobalLogger.Debug("Added default header", "header", "X-Content-Type-Options", "value", "nosniff")
	cfgOpenAPI.AddDefaultHeader("X-Frame-Options", "DENY")
	config.GlobalLogger.Debug("Added default header", "header", "X-Frame-Options", "value", "DENY")
	cfgOpenAPI.AddDefaultHeader("X-Atlassian-Token", "no-check")
	config.GlobalLogger.Debug("Added default header", "header", "X-Atlassian-Token", "value", "no-check")

	if config.GlobalCfg == nil {
		return nil, fmt.Errorf("config is nil")
	}
	if config.GlobalLogger == nil {
		return nil, fmt.Errorf("logger is nil")
	}

	config.GlobalLogger.Debug("Checking authentication configuration")
	authCtx := ctx
	if config.GlobalCfg.Username != "" && config.GlobalCfg.Password != "" {
		config.GlobalLogger.Debug("Using username/password for basic auth", "username", config.GlobalCfg.Username)
		authCtx = context.WithValue(ctx, openapi.ContextBasicAuth, openapi.BasicAuth{
			UserName: config.GlobalCfg.Username,
			Password: config.GlobalCfg.Password,
		})
		config.GlobalLogger.Debug("Using Basic Auth")
	} else if config.GlobalCfg.Token != "" {
		config.GlobalLogger.Debug("Using Bearer token authentication")
		cfgOpenAPI.AddDefaultHeader("Authorization", "Bearer "+config.GlobalCfg.Token)
	} else {
		config.GlobalLogger.Error("No valid authentication credentials provided")
		return nil, fmt.Errorf("either token or username/password must be provided")
	}

	config.GlobalLogger.Debug("Bitbucket client successfully initialized")
	return &Client{
		api:     openapi.NewAPIClient(cfgOpenAPI),
		logger:  config.GlobalLogger,
		client:  &http.Client{},
		authCtx: authCtx,
		config:  config.GlobalCfg,
		Logger:  config.GlobalLogger,
	}, nil
}
