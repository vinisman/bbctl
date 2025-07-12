package create

import (
	"sync"

	"github.com/spf13/cobra"

	"github.com/vinisman/bbctl/api"
	"github.com/vinisman/bbctl/models"
	"github.com/vinisman/bbctl/utils"
)

var BuildsCmd = &cobra.Command{
	Use:   "required-builds",
	Short: "Create required builds",
	Run: func(cmd *cobra.Command, args []string) {
		cfg := utils.LoadConfig()

		repos := utils.ReadRepositoryWithBuildsYaml(utils.InputFile)
		svc := api.NewRequiredBuildsService(cfg, utils.ProjectKey)

		var wg sync.WaitGroup
		sem := make(chan struct{}, 5)

		for _, repo := range repos {
			wg.Add(1)
			go func(r models.RepoWithBuildKeys) {
				defer wg.Done()
				sem <- struct{}{}
				defer func() { <-sem }()
				svc.CreateCondition(repo.Branch, repo.BuildKeys, repo.Name)
			}(repo)
		}
		wg.Wait()
	},
}

func init() {
	BuildsCmd.Flags().StringVarP(&utils.InputFile, "input", "i", "", "YAML file with list of repositories")
	BuildsCmd.MarkFlagRequired("input")
}
