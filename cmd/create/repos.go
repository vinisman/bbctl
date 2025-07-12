package create

import (
	"sync"

	"github.com/spf13/cobra"

	"github.com/vinisman/bbctl/api"
	"github.com/vinisman/bbctl/models"
	"github.com/vinisman/bbctl/utils"
)

var ReposCmd = &cobra.Command{
	Use:   "repos",
	Short: "Bulk create repos if not exist (skip if exist)",
	Run: func(cmd *cobra.Command, args []string) {
		cfg := utils.LoadConfig()

		repos := utils.ReadRepositoryYaml(utils.InputFile)
		svc := api.NewRepoService(cfg, utils.ProjectKey)
		var wg sync.WaitGroup
		sem := make(chan struct{}, 5)

		for _, repo := range repos {
			wg.Add(1)
			go func(r models.RepoEntity) {
				defer wg.Done()
				sem <- struct{}{}
				defer func() { <-sem }()
				svc.Create(repo)
			}(repo)
		}
		wg.Wait()
	},
}

func init() {
	ReposCmd.Flags().StringVarP(&utils.InputFile, "input", "i", "", "YAML file with list of repositories")
}
