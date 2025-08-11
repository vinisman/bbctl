package apply

import (
	"context"
	"fmt"
	"net/http"

	"github.com/spf13/cobra"
	"github.com/vinisman/bbctl/internal"
	"github.com/vinisman/bbctl/utils"
	openapi "github.com/vinisman/bitbucket-sdk-go/openapi"
)

var (
	repoName   string
	repoSlug   string
	repoBranch string
)

var applyRepoCmd = &cobra.Command{
	Use:   "repo",
	Short: "Create or update a single repository",
	Long: `Create or update a Bitbucket repository. 
If the repository exists, it will be updated; otherwise, it will be created.

Flags:
  --name        Repository display name
  --slug        Repository slug (URL-friendly name, lowercase, no spaces)
  --branch      Default branch name (default: main)
  --project     Bitbucket project key (or env BITBUCKET_PROJECT_KEY)

Example:
  bbctl apply repo --name "My Repo" --slug my-repo --branch main -p PROJECT
`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if utils.ProjectKey == "" {
			return fmt.Errorf("parameter BITBUCKET_PROJECT_KEY or --project must be set")
		}
		if repoSlug == "" {
			return fmt.Errorf("--slug must be set")
		}
		if repoName == "" {
			return fmt.Errorf("--name must be set")
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

		// Проверяем, существует ли репозиторий
		_, resp, err := client.ProjectAPI.GetRepository(ctx, utils.ProjectKey, repoSlug).Execute()
		if err != nil {
			if resp != nil && resp.StatusCode == http.StatusNotFound {
				utils.Logger.Info("Repository not found, creating...", "slug", repoSlug)
				_, _, err := client.ProjectAPI.CreateRepository(ctx, utils.ProjectKey).RestRepository(req).Execute()
				if err != nil {
					utils.Logger.Error("Failed to create repository", "error", err)
					return fmt.Errorf("failed to create repository: %w", err)
				}
				utils.Logger.Info("Repository created successfully", "name", repoName, "slug", repoSlug, "branch", repoBranch)
				return nil
			}
			utils.Logger.Error("Failed to check repository existence", "error", err)
			return fmt.Errorf("failed to get repository: %w", err)
		}

		// Обновление
		utils.Logger.Info("Repository exists, updating...", "slug", repoSlug)
		_, _, err = client.ProjectAPI.UpdateRepository(ctx, utils.ProjectKey, repoSlug).RestRepository(req).Execute()
		if err != nil {
			utils.Logger.Error("Failed to update repository", "error", err)
			return fmt.Errorf("failed to update repository: %w", err)
		}
		utils.Logger.Info("Repository updated successfully", "name", repoName, "slug", repoSlug, "branch", repoBranch)
		return nil
	},
}

func init() {
	applyRepoCmd.PersistentFlags().StringVar(&repoName, "name", "", "Repository display name")
	applyRepoCmd.PersistentFlags().StringVarP(&repoSlug, "slug", "s", "", "Repository slug (URL-friendly name)")
	applyRepoCmd.PersistentFlags().StringVarP(&repoBranch, "branch", "b", "main", "Default repository branch")
	applyRepoCmd.PersistentFlags().StringVarP(&utils.ProjectKey, "project", "p", "", "Bitbucket project key (or env BITBUCKET_PROJECT_KEY)")
}
