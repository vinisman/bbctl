package delete

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
	"github.com/vinisman/bbctl/internal"
	"github.com/vinisman/bbctl/utils"
)

var (
	repoSlug   string
	repoBranch string
)

var deleteRepoCmd = &cobra.Command{
	Use:   "repo [slug]",
	Short: "Delete a single repository",
	Long: `Delete a single Bitbucket repository.

Positional argument:
  slug          Repository slug

Flags:
  --project     Bitbucket project key (or env BITBUCKET_PROJECT_KEY)

Example:
  bbctl delete repo my-repo -p PROJECT
`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		repoSlug := args[0]

		if utils.ProjectKey == "" {
			return fmt.Errorf("parameter BITBUCKET_PROJECT_KEY or --project must be set")
		}

		client := internal.NewClient(utils.Cfg.BitbucketURL, utils.Cfg.BitbucketToken, utils.Logger)
		ctx := context.Background()

		_, err := client.ProjectAPI.DeleteRepository(ctx, utils.ProjectKey, repoSlug).Execute()
		if err != nil {
			utils.Logger.Error("Failed to delete repository", "error", err)
			return fmt.Errorf("failed to delete repository: %w", err)
		}

		utils.Logger.Info("Repository deleted successfully", "slug", repoSlug)
		return nil
	},
}

func init() {
	deleteRepoCmd.PersistentFlags().StringVarP(&utils.ProjectKey, "project", "p", "", "Bitbucket project key (or env BITBUCKET_PROJECT_KEY)")
}
