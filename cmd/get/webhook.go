package get

import (
	"context"
	"encoding/json"
	"fmt"
	"io"

	"github.com/spf13/cobra"
	"github.com/vinisman/bbctl/internal"
	"github.com/vinisman/bbctl/utils"
)

var (
	repoWebhookSlug string
)

var getRepoWebhooksCmd = &cobra.Command{
	Use:   "repo-webhook",
	Short: "List all webhooks for a specific repository",
	Long: `List all webhooks for a specified Bitbucket repository.

Flags:
  --project    Bitbucket project key (or env BITBUCKET_PROJECT_KEY)
  --slug       Repository slug (required)

Example:
  bbctl get repo-webhook --project PRJ --slug my-repo
`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if utils.ProjectKey == "" {
			return fmt.Errorf("parameter BITBUCKET_PROJECT_KEY or --project must be set")
		}
		if repoWebhookSlug == "" {
			return fmt.Errorf("--slug is required")
		}

		client := internal.NewClient(utils.Cfg.BitbucketURL, utils.Cfg.BitbucketToken, utils.Logger)
		ctx := context.Background()

		httpResp, err := client.RepositoryAPI.FindWebhooks1(ctx, utils.ProjectKey, repoWebhookSlug).Execute()
		if err != nil {
			utils.Logger.Error("Failed to get repository webhooks", "error", err)
			return fmt.Errorf("failed to get repository webhooks: %w", err)
		}

		defer httpResp.Body.Close()
		bodyBytes, err := io.ReadAll(httpResp.Body)
		if err != nil {
			utils.Logger.Error("Failed to read response body", "error", err)
			return err
		}

		var body struct {
			Values []struct {
				Id                      int      `json:"id"`
				Name                    string   `json:"name"`
				Url                     string   `json:"url"`
				Events                  []string `json:"events"`
				Active                  bool     `json:"active"`
				SslVerificationRequired bool     `json:"sslVerificationRequired"`
			} `json:"values"`
		}

		if err := json.Unmarshal(bodyBytes, &body); err != nil {
			utils.Logger.Error("Failed to parse response body", "error", err)
			return err
		}

		webhooks := []internal.WebhookInfo{}
		for _, wh := range body.Values {
			webhooks = append(webhooks, internal.WebhookInfo{
				Project:                 utils.ProjectKey,
				Slug:                    repoWebhookSlug,
				Name:                    wh.Name,
				URL:                     wh.Url,
				Events:                  wh.Events,
				Active:                  wh.Active,
				SslVerificationRequired: wh.SslVerificationRequired,
				// Username, Password
			})
		}

		internal.PrintWebhooks(webhooks, OutputFormat) // format может быть "table", "yaml", "json"

		utils.Logger.Debug("Listed repository webhooks", "repo", repoWebhookSlug, "count", len(body.Values))
		return nil
	},
}

func joinEvents(events []string) string {
	return fmt.Sprintf("%v", events)
}

func init() {
	getRepoWebhooksCmd.PersistentFlags().StringVarP(&utils.ProjectKey, "project", "p", "", "Bitbucket project key (or env BITBUCKET_PROJECT_KEY)")
	getRepoWebhooksCmd.PersistentFlags().StringVar(&repoWebhookSlug, "slug", "", "Repository slug (required)")
}
