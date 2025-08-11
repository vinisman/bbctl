package apply

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
	"github.com/vinisman/bbctl/internal"
	"github.com/vinisman/bbctl/utils"
	openapi "github.com/vinisman/bitbucket-sdk-go/openapi"
)

var (
	repoWebhookSlug        string
	webhookName            string
	webhookURL             string
	webhookEvents          []string
	webhookActive          bool
	webhookUsername        string
	webhookPassword        string
	webhookSSLVerification bool
)

var applyRepoWebhookCmd = &cobra.Command{
	Use:   "repo-webhook",
	Short: "Create or update a webhook for a specific repository",
	Long: `Create or update a webhook for a specified Bitbucket repository.

Flags:
  --project        Bitbucket project key (or env BITBUCKET_PROJECT_KEY)
  --slug           Repository slug (required)
  --name           Webhook name (required)
  --url            Webhook target URL (required)
  --events         Comma-separated list of events, e.g., repo:refs_changed,pr:opened (required)
  --active         Whether the webhook is active (default: true)
  --username       Username for HTTP basic auth
  --password       Password for HTTP basic auth
  --ssl-verification  Whether to verify SSL certificate (default: true)

Example:
  bbctl apply repo-webhook --project PRJ --slug my-repo --name "CI Hook" \
    --url https://ci.example.com/hook --events repo:refs_changed,pr:opened \
    --active=true --username user --password pass --ssl-verification=false
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
		if webhookURL == "" {
			return fmt.Errorf("--url is required")
		}
		if len(webhookEvents) == 0 {
			return fmt.Errorf("--events is required")
		}

		client := internal.NewClient(utils.Cfg.BitbucketURL, utils.Cfg.BitbucketToken, utils.Logger)
		ctx := context.Background()

		// Ищем webhook по имени
		webhookID, err := internal.FindWebhookIDByName(ctx, client, utils.ProjectKey, repoWebhookSlug, webhookName)
		if err != nil {
			utils.Logger.Error("Failed to find existing webhook", "error", err)
			return fmt.Errorf("failed to find existing webhook: %w", err)
		}

		req := openapi.RestWebhook{
			Name:                    &webhookName,
			Url:                     &webhookURL,
			Events:                  webhookEvents,
			Active:                  openapi.PtrBool(webhookActive),
			SslVerificationRequired: openapi.PtrBool(webhookSSLVerification),
			ScopeType:               openapi.PtrString("repository"),
		}

		if webhookUsername != "" || webhookPassword != "" {
			req.Credentials = &openapi.RestWebhookCredentials{
				Username: &webhookUsername,
				Password: &webhookPassword,
			}
		}

		if webhookID != "" {
			// Update webhook
			_, _, err := client.RepositoryAPI.UpdateWebhook1(ctx, utils.ProjectKey, webhookID, repoWebhookSlug).
				RestWebhook(req).
				Execute()
			if err != nil {
				utils.Logger.Error("Failed to update repository webhook", "error", err)
				return fmt.Errorf("failed to update repository webhook: %w", err)
			}
			utils.Logger.Info("Repository webhook updated successfully",
				"project", utils.ProjectKey,
				"repo", repoWebhookSlug,
				"name", webhookName,
			)
		} else {
			// Create webhook
			_, _, err := client.RepositoryAPI.CreateWebhook1(ctx, utils.ProjectKey, repoWebhookSlug).
				RestWebhook(req).
				Execute()
			if err != nil {
				utils.Logger.Error("Failed to create repository webhook", "error", err)
				return fmt.Errorf("failed to create repository webhook: %w", err)
			}
			utils.Logger.Info("Repository webhook created successfully",
				"project", utils.ProjectKey,
				"repo", repoWebhookSlug,
				"name", webhookName,
			)
		}

		return nil
	},
}

func init() {
	applyRepoWebhookCmd.PersistentFlags().StringVarP(&utils.ProjectKey, "project", "p", "", "Bitbucket project key (or env BITBUCKET_PROJECT_KEY)")
	applyRepoWebhookCmd.PersistentFlags().StringVar(&repoWebhookSlug, "slug", "", "Repository slug (required)")
	applyRepoWebhookCmd.PersistentFlags().StringVar(&webhookName, "name", "", "Webhook display name (required)")
	applyRepoWebhookCmd.PersistentFlags().StringVar(&webhookURL, "url", "", "Webhook target URL (required)")
	applyRepoWebhookCmd.PersistentFlags().StringSliceVar(&webhookEvents, "events", nil, "Comma-separated list of webhook events (required)")
	applyRepoWebhookCmd.PersistentFlags().BoolVar(&webhookActive, "active", true, "Whether the webhook should be active")
	applyRepoWebhookCmd.PersistentFlags().StringVar(&webhookUsername, "username", "", "HTTP basic auth username")
	applyRepoWebhookCmd.PersistentFlags().StringVar(&webhookPassword, "password", "", "HTTP basic auth password")
	applyRepoWebhookCmd.PersistentFlags().BoolVar(&webhookSSLVerification, "ssl-verification", true, "Require SSL verification")
}
