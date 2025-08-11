package create

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
	"github.com/vinisman/bbctl/internal"
	"github.com/vinisman/bbctl/utils"
	openapi "github.com/vinisman/bitbucket-sdk-go/openapi"
)

var (
	repoSlug   string
	repoBranch string
)

var createRepoCmd = &cobra.Command{
	Use:   "repo [name]",
	Short: "Create a single repository",
	Long: `Create a single Bitbucket repository.

Positional argument:
  name          Repository display name

Flags:
  --slug        Repository slug (URL-friendly name, lowercase, no spaces)
  --branch      Initial branch name (default: main)
  --project     Bitbucket project key (or env BITBUCKET_PROJECT_KEY)

Example:
  bbctl create repo "My Repo" --slug my-repo --branch main -p PROJECT
`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		repoName := args[0]

		if utils.ProjectKey == "" {
			return fmt.Errorf("parameter BITBUCKET_PROJECT_KEY or --project must be set")
		}

		client := internal.NewClient(utils.Cfg.BitbucketURL, utils.Cfg.BitbucketToken, utils.Logger)
		ctx := context.Background()

		scmId := "git"
		req := openapi.RestRepository{
			Name:          &repoName,
			Slug:          &repoSlug,
			ScmId:         &scmId,
			DefaultBranch: &repoBranch,
		}

		_, _, err := client.ProjectAPI.CreateRepository(ctx, utils.ProjectKey).RestRepository(req).Execute()
		if err != nil {
			utils.Logger.Error("Failed to create repository", "error", err)
			return fmt.Errorf("failed to create repository: %w", err)
		}

		utils.Logger.Info("Repository created successfully", "name", repoName, "slug", repoSlug, "branch", repoBranch)
		return nil
	},
}

func init() {
	createRepoCmd.PersistentFlags().StringVarP(&repoSlug, "slug", "s", "", "Repository slug (URL-friendly name)")
	createRepoCmd.PersistentFlags().StringVarP(&repoBranch, "branch", "b", "main", "Default repository branch")
	createRepoCmd.PersistentFlags().StringVarP(&utils.ProjectKey, "project", "p", "", "Bitbucket project key (or env BITBUCKET_PROJECT_KEY)")
}
