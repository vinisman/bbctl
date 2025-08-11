package get

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
	"github.com/vinisman/bbctl/internal"
	"github.com/vinisman/bbctl/utils" // чтобы получить глобальную cfg
)

var getRepoCmd = &cobra.Command{
	Use:   "repo [slug]",
	Short: "Get information about a specific repository by slug",
	Args:  cobra.ExactArgs(1),
	Example: `
  bbctl get repo my-repo -p PROJECT
	`,
	RunE: func(cmd *cobra.Command, args []string) error {

		if utils.ProjectKey == "" {
			return fmt.Errorf("parameter BITBUCKET_PROJECT_KEY or --project must be set")
		}

		slug := args[0]
		if slug == "" {
			return fmt.Errorf("repository slug must be specified")
		}

		client := internal.NewClient(utils.Cfg.BitbucketURL, utils.Cfg.BitbucketToken, utils.Logger)

		ctx := context.Background()
		repo, httpResp, err := client.ProjectAPI.GetRepository(ctx, utils.Cfg.ProjectKey, slug).Execute()
		if err != nil {
			cmd.SilenceUsage = true
			return fmt.Errorf("failed to get repository: %w", err)
		}

		if err := internal.CheckAuthByHeader(httpResp); err != nil {
			utils.Logger.Error(err.Error())
			return err
		}
		repoSlice := utils.SingleRepoSlice(repo)

		cols := utils.ParseColumns(Columns)
		needDefaultBranch := utils.Contains(cols, "DefaultBranch")

		if needDefaultBranch {
			internal.FetchDefaultBranches(client, repoSlice)
		}

		manifestFieldsList := utils.ParseColumns(ManifestFields)
		needManifest := ManifestFile != "" && len(manifestFieldsList) > 0
		var manifestDataMap map[string]map[string]string
		if needManifest {
			manifestDataMap = internal.FetchManifests(client, repoSlice, ManifestFile, manifestFieldsList)
		}

		if ManifestExist && needManifest {
			filtered := repoSlice[:0]
			for _, r := range repoSlice {
				if _, ok := manifestDataMap[utils.DerefString(r.Slug)]; ok {
					filtered = append(filtered, r)
				}
			}
			repoSlice = filtered
		}
		internal.PrintRepos(repoSlice, Columns, manifestFieldsList, manifestDataMap, OutputFormat)
		return nil
	},
}
