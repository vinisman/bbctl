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
	if config.GlobalCfg == nil {
		return nil, fmt.Errorf("config is nil")
	}
	if config.GlobalLogger == nil {
		return nil, fmt.Errorf("logger is nil")
	}

	cfgOpenAPI := openapi.NewConfiguration()
	cfgOpenAPI.Servers = openapi.ServerConfigurations{{URL: config.GlobalCfg.BaseURL}}

	authCtx := ctx
	if config.GlobalCfg.Username != "" && config.GlobalCfg.Password != "" {
		authCtx = context.WithValue(ctx, openapi.ContextBasicAuth, openapi.BasicAuth{
			UserName: config.GlobalCfg.Username,
			Password: config.GlobalCfg.Password,
		})
		config.GlobalLogger.Debug("Using Basic Auth")
	} else if config.GlobalCfg.Token != "" {
		cfgOpenAPI.AddDefaultHeader("Authorization", "Bearer "+config.GlobalCfg.Token)
		config.GlobalLogger.Debug("Using Token Auth")
	} else {
		return nil, fmt.Errorf("either token or username/password must be provided")
	}

	return &Client{
		api:     openapi.NewAPIClient(cfgOpenAPI),
		logger:  config.GlobalLogger,
		client:  &http.Client{},
		authCtx: authCtx,
		config:  config.GlobalCfg,
		Logger:  config.GlobalLogger,
	}, nil
}
