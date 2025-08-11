package get

import (
	"context"
	"fmt"
	"net/http"

	"github.com/spf13/cobra"
	"github.com/vinisman/bbctl/internal"
	"github.com/vinisman/bbctl/utils"
	openapi "github.com/vinisman/bitbucket-sdk-go/openapi"
)

var getReposCmd = &cobra.Command{
	Use:   "repos",
	Short: "List all repositories in a project",
	RunE: func(cmd *cobra.Command, args []string) error {

		if utils.ProjectKey == "" && utils.AllProjects == false {
			return fmt.Errorf("parameter BITBUCKET_PROJECT_KEY or --project or --all-projects must be set")
		}
		client := internal.NewClient(utils.Cfg.BitbucketURL, utils.Cfg.BitbucketToken, utils.Logger)

		ctx := context.Background()
		var allRepos []openapi.RestRepository
		start := float32(0)

		for {
			var repos *openapi.GetRepositoriesRecentlyAccessed200Response
			var err error
			var httpResp *http.Response
			if utils.AllProjects {
				repos, httpResp, err = client.RepositoryAPI.GetRepositories1(ctx).
					Start(start).
					Limit(float32(utils.Cfg.PageSize)).
					Execute()
			} else {
				repos, httpResp, err = client.ProjectAPI.GetRepositories(ctx, utils.Cfg.ProjectKey).
					Start(start).
					Limit(float32(utils.Cfg.PageSize)).
					Execute()
			}
			if err != nil {
				utils.Logger.Error("Failed to get repositories", "error", err)
				return err
			}

			if err := internal.CheckAuthByHeader(httpResp); err != nil {
				utils.Logger.Error(err.Error())
				return err
			}

			allRepos = append(allRepos, repos.Values...)

			if repos.NextPageStart != nil {
				start = float32(*repos.NextPageStart)
			} else {
				break
			}
		}

		cols := utils.ParseColumns(Columns)
		needDefaultBranch := utils.Contains(cols, "DefaultBranch")

		if needDefaultBranch {
			internal.FetchDefaultBranches(client, allRepos)
		}

		manifestFieldsList := utils.ParseColumns(ManifestFields)
		needManifest := ManifestFile != "" && len(manifestFieldsList) > 0
		var manifestDataMap map[string]map[string]string
		if needManifest {
			manifestDataMap = internal.FetchManifests(client, allRepos, ManifestFile, manifestFieldsList)
		}

		if ManifestExist && needManifest {
			filtered := allRepos[:0]
			for _, r := range allRepos {
				if _, ok := manifestDataMap[utils.DerefString(r.Slug)]; ok {
					filtered = append(filtered, r)
				}
			}
			allRepos = filtered
		}

		utils.PrintRepos(allRepos, Columns, manifestFieldsList, manifestDataMap, OutputFormat)

		utils.Logger.Debug("Listed repositories", "count", len(allRepos))
		return nil
	},
}

func init() {
	getReposCmd.Flags().BoolVar(&utils.AllProjects, "all-projects", false, "Bitbucket all projects search")
}
