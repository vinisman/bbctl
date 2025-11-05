package workzone

import (
	"context"
	"strings"

	bb "github.com/vinisman/bbctl/internal/bitbucket"
	wz "github.com/vinisman/workzone-sdk-go/client"
)

type Client struct {
	api *wz.APIClient
	ctx context.Context
}

// NewClient constructs Workzone client based on existing Bitbucket client configuration
func NewClient(bc *bb.Client) *Client {
	cfg := wz.NewConfiguration()
	base := bc.API().GetConfig().Servers[0].URL
	base = strings.TrimRight(base, "/") + "/workzoneresource/latest"
	cfg.Servers = wz.ServerConfigurations{wz.ServerConfiguration{URL: base}}

	// Copy only essential headers from Bitbucket client
	if userAgent := bc.API().GetConfig().DefaultHeader["User-Agent"]; userAgent != "" {
		cfg.AddDefaultHeader("User-Agent", userAgent)
	}
	if authHeader := bc.API().GetConfig().DefaultHeader["Authorization"]; authHeader != "" {
		cfg.AddDefaultHeader("Authorization", authHeader)
	}
	if atlassianToken := bc.API().GetConfig().DefaultHeader["X-Atlassian-Token"]; atlassianToken != "" {
		cfg.AddDefaultHeader("X-Atlassian-Token", atlassianToken)
	}

	// Add JSON-specific headers for Workzone API
	cfg.AddDefaultHeader("Accept", "application/json")
	// Content-Type should be set by the SDK automatically for each request

	if bc.API().GetConfig().HTTPClient != nil {
		cfg.HTTPClient = bc.API().GetConfig().HTTPClient
	}

	return &Client{api: wz.NewAPIClient(cfg), ctx: bc.Context()}
}

func (c *Client) API() *wz.APIClient       { return c.api }
func (c *Client) Context() context.Context { return c.ctx }
