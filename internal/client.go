package internal

import (
	"fmt"
	"log/slog"
	"net/http"

	openapi "github.com/vinisman/bitbucket-sdk-go/openapi"
)

func NewClient(baseURL string, token string, logger *slog.Logger) *openapi.APIClient {
	cfg := openapi.NewConfiguration()
	cfg.Servers = openapi.ServerConfigurations{
		{URL: baseURL},
	}
	cfg.AddDefaultHeader("Authorization", "Bearer "+token)

	// Currently no special HTTP client or logging on requests
	client := openapi.NewAPIClient(cfg)
	return client
}

func CheckAuthByHeader(httpResp *http.Response) error {
	username := httpResp.Header.Get("X-AUSERNAME")
	if username == "" || username == "anonymous" {
		return fmt.Errorf("authentication failed: no valid credentials provided")
	}
	return nil
}
