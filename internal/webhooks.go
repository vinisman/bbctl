package internal

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/vinisman/bbctl/utils"
	openapi "github.com/vinisman/bitbucket-sdk-go/openapi"
)

type WebhookResponse struct {
	Size       int                `json:"size"`
	Limit      int                `json:"limit"`
	IsLastPage bool               `json:"isLastPage"`
	Values     []WebhookShortInfo `json:"values"`
	Start      int                `json:"start"`
}

type WebhookShortInfo struct {
	Id                      uint32 `json:"id"`
	Name                    string `json:"name"`
	Url                     string `json:"url"`
	Active                  bool   `json:"active"`
	ScopeType               string `json:"scopeType"`
	SslVerificationRequired bool   `json:"sslVerificationRequired"`
}

// FindWebhookIDByName serachs webhook by name and return ID or empty if not found
func FindWebhookIDByName(ctx context.Context, client *openapi.APIClient, projectKey, repoSlug, webhookName string) (string, error) {
	httpResp, err := client.RepositoryAPI.FindWebhooks1(ctx, projectKey, repoSlug).Execute()
	if err != nil {
		return "", fmt.Errorf("failed to call FindWebhooks1: %w", err)
	}
	defer httpResp.Body.Close()

	if httpResp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(httpResp.Body)
		return "", fmt.Errorf("unexpected status %d: %s", httpResp.StatusCode, string(bodyBytes))
	}

	bodyBytes, err := io.ReadAll(httpResp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read response body: %w", err)
	}

	utils.Logger.Debug("Details", "bodyBytes", bodyBytes)

	var webhooksResp WebhookResponse
	if err := json.Unmarshal(bodyBytes, &webhooksResp); err != nil {
		return "", fmt.Errorf("failed to parse webhook response JSON: %w", err)
	}

	for _, wh := range webhooksResp.Values {
		if wh.Name == webhookName {
			webhookId := fmt.Sprint(wh.Id)
			utils.Logger.Debug("Details", "webhookId", webhookId)
			return webhookId, nil
		}
	}

	return "", nil
}
