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

	for k, v := range bc.API().GetConfig().DefaultHeader {
		cfg.AddDefaultHeader(k, v)
	}
	if bc.API().GetConfig().HTTPClient != nil {
		cfg.HTTPClient = bc.API().GetConfig().HTTPClient
	}

	return &Client{api: wz.NewAPIClient(cfg), ctx: bc.Context()}
}

func (c *Client) API() *wz.APIClient       { return c.api }
func (c *Client) Context() context.Context { return c.ctx }
