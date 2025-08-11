package internal

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"text/tabwriter"

	"github.com/vinisman/bbctl/utils"
	openapi "github.com/vinisman/bitbucket-sdk-go/openapi"
	"gopkg.in/yaml.v2"
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

// WebhookInfo
type WebhookInfo struct {
	Project                 string   `json:"project" yaml:"project"`
	Slug                    string   `json:"slug" yaml:"slug"`
	Name                    string   `json:"name" yaml:"name"`
	URL                     string   `json:"url" yaml:"url"`
	Events                  []string `json:"events" yaml:"events"`
	Active                  bool     `json:"active" yaml:"active"`
	Username                string   `json:"username,omitempty" yaml:"username,omitempty"`
	Password                string   `json:"password,omitempty" yaml:"password,omitempty"`
	SslVerificationRequired bool     `json:"sslVerificationRequired" yaml:"sslVerificationRequired"`
}

// PrintWebhooks
func PrintWebhooks(webhooks []WebhookInfo, format string) {
	switch strings.ToLower(format) {
	case "yaml":
		wrapped := map[string]interface{}{
			"webhooks": webhooks,
		}
		out, err := yaml.Marshal(wrapped)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error generating YAML: %v\n", err)
			os.Exit(1)
		}
		fmt.Print(string(out))

	case "json":
		wrapped := map[string]interface{}{
			"webhooks": webhooks,
		}
		out, err := json.MarshalIndent(wrapped, "", "  ")
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error generating JSON: %v\n", err)
			os.Exit(1)
		}
		fmt.Print(string(out))

	default: // plain
		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		fmt.Fprintln(w, "PROJECT\tSLUG\tNAME\tURL\tEVENTS\tACTIVE\tSSL VERIFICATION")
		for _, wh := range webhooks {
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%t\t%t\n",
				wh.Project,
				wh.Slug,
				wh.Name,
				wh.URL,
				strings.Join(wh.Events, ","),
				wh.Active,
				wh.SslVerificationRequired,
			)
		}
		w.Flush()
	}
}
