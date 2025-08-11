package delete

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
	"github.com/vinisman/bbctl/internal"
	"github.com/vinisman/bbctl/utils"
)

var (
	repoWebhookSlug string
	webhookName     string
)

var deleteRepoWebhookCmd = &cobra.Command{
	Use:   "repo-webhook",
	Short: "Delete a webhook for a specific repository",
	Long: `Delete a webhook from a specified Bitbucket repository by webhook name.

Flags:
  --project    Bitbucket project key (or env BITBUCKET_PROJECT_KEY)
  --slug       Repository slug (required)
  --name       Webhook name (required)

Example:
  bbctl delete repo-webhook --project PRJ --slug my-repo --name "CI Hook"
`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if utils.ProjectKey == "" {
			return fmt.Errorf("parameter BITBUCKET_PROJECT_KEY or --project must be set")
		}
		if repoWebhookSlug == "" {
			return fmt.Errorf("--slug is required")
		}
		if webhookName == "" {
			return fmt.Errorf("--name is required")
		}

		client := internal.NewClient(utils.Cfg.BitbucketURL, utils.Cfg.BitbucketToken, utils.Logger)
		ctx := context.Background()

		webhookID, err := internal.FindWebhookIDByName(ctx, client, utils.ProjectKey, repoWebhookSlug, webhookName)
		if err != nil {
			return fmt.Errorf("failed to find webhook: %w", err)
		}
		if webhookID == "" {
			utils.Logger.Info("Webhook not found, nothing to delete", "name", webhookName, "repo", repoWebhookSlug)
			return nil
		}

		httpResp, err := client.RepositoryAPI.DeleteWebhook1(ctx, utils.ProjectKey, webhookID, repoWebhookSlug).Execute()
		if err != nil {
			utils.Logger.Error("Failed to delete repository webhook", "error", err)
			utils.Logger.Debug("Details", "httpResp", httpResp.Body)
			return fmt.Errorf("failed to delete repository webhook: %w", err)
		}

		utils.Logger.Info("Repository webhook deleted successfully",
			"project", utils.ProjectKey,
			"repo", repoWebhookSlug,
			"name", webhookName,
		)
		return nil
	},
}

func init() {
	deleteRepoWebhookCmd.PersistentFlags().StringVarP(&utils.ProjectKey, "project", "p", "", "Bitbucket project key (or env BITBUCKET_PROJECT_KEY)")
	deleteRepoWebhookCmd.PersistentFlags().StringVar(&repoWebhookSlug, "slug", "", "Repository slug (required)")
	deleteRepoWebhookCmd.PersistentFlags().StringVar(&webhookName, "name", "", "Webhook display name (required)")
}
