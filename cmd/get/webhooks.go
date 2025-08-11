package get

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"text/tabwriter"

	"github.com/spf13/cobra"
	"github.com/vinisman/bbctl/internal"
	"github.com/vinisman/bbctl/utils"
)

var (
	repoWebhookSlug string
)

var getRepoWebhooksCmd = &cobra.Command{
	Use:   "repo-webhooks",
	Short: "List all webhooks for a specific repository",
	Long: `List all webhooks for a specified Bitbucket repository.

Flags:
  --project    Bitbucket project key (or env BITBUCKET_PROJECT_KEY)
  --slug       Repository slug (required)

Example:
  bbctl get repo-webhooks --project PRJ --slug my-repo
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

		w := tabwriter.NewWriter(os.Stdout, 5, 2, 2, ' ', 0)
		fmt.Fprintf(w, "ID\tNAME\tURL\tEVENTS\tACTIVE\tSSL Verification\tREPO\n")
		for _, wh := range body.Values {
			fmt.Fprintf(w, "%d\t%s\t%s\t%s\t%t\t%t\t%s\n",
				wh.Id,
				wh.Name,
				wh.Url,
				joinEvents(wh.Events),
				wh.Active,
				wh.SslVerificationRequired,
				repoWebhookSlug,
			)
		}
		w.Flush()

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
