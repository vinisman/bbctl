package create

import (
	"context"
	"fmt"
	"strings"

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

var createRepoWebhookCmd = &cobra.Command{
	Use:   "repo-webhook",
	Short: "Create a webhook for a specific repository",
	Long: `Create a webhook for a specified Bitbucket repository.

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
  bbctl create repo-webhook --project PRJ --slug my-repo --name "CI Hook" \
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
		scopeType := "repository"

		req := openapi.RestWebhook{
			Name:                    &webhookName,
			Url:                     &webhookURL,
			Events:                  webhookEvents,
			Active:                  openapi.PtrBool(webhookActive),
			SslVerificationRequired: openapi.PtrBool(webhookSSLVerification),
			ScopeType:               &scopeType,
		}

		if webhookUsername != "" || webhookPassword != "" {
			req.Credentials = &openapi.RestWebhookCredentials{
				Username: &webhookUsername,
				Password: &webhookPassword,
			}
		}

		_, httpResp, err := client.RepositoryAPI.CreateWebhook1(ctx, utils.ProjectKey, repoWebhookSlug).
			RestWebhook(req).
			Execute()
		if err != nil {
			utils.Logger.Error("Failed to create repository webhook", "error", err)
			return fmt.Errorf("failed to create repository webhook: %w", err)
		}
		utils.Logger.Debug("Details", "httpResp", httpResp)

		utils.Logger.Info("Repository webhook created successfully",
			"project", utils.ProjectKey,
			"repo", repoWebhookSlug,
			"name", webhookName,
			"url", webhookURL,
			"events", strings.Join(webhookEvents, ","),
			"active", webhookActive,
			"sslVerificationRequired", webhookSSLVerification,
		)
		return nil
	},
}

func init() {
	createRepoWebhookCmd.PersistentFlags().StringVarP(&utils.ProjectKey, "project", "p", "", "Bitbucket project key (or env BITBUCKET_PROJECT_KEY)")
	createRepoWebhookCmd.PersistentFlags().StringVar(&repoWebhookSlug, "slug", "", "Repository slug (required)")
	createRepoWebhookCmd.PersistentFlags().StringVar(&webhookName, "name", "", "Webhook display name (required)")
	createRepoWebhookCmd.PersistentFlags().StringVar(&webhookURL, "url", "", "Webhook target URL (required)")
	createRepoWebhookCmd.PersistentFlags().StringSliceVar(&webhookEvents, "events", nil, "Comma-separated list of webhook events (required)")
	createRepoWebhookCmd.PersistentFlags().BoolVar(&webhookActive, "active", true, "Whether the webhook should be active")
	createRepoWebhookCmd.PersistentFlags().StringVar(&webhookUsername, "username", "", "HTTP basic auth username")
	createRepoWebhookCmd.PersistentFlags().StringVar(&webhookPassword, "password", "", "HTTP basic auth password")
	createRepoWebhookCmd.PersistentFlags().BoolVar(&webhookSSLVerification, "ssl-verification", true, "Require SSL verification")
}
