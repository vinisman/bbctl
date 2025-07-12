package delete

import (
	"sync"

	"github.com/spf13/cobra"
	"github.com/vinisman/bbctl/api"
	"github.com/vinisman/bbctl/models"
	"github.com/vinisman/bbctl/utils"
)

var BuildsCmd = &cobra.Command{
	Use:   "required-builds",
	Short: "Bulk delete required-builds",
	Run: func(cmd *cobra.Command, args []string) {

		cfg := utils.LoadConfig()
		repos := utils.ReadRepositoryYaml(utils.InputFile)
		svc := api.NewRequiredBuildsService(cfg, utils.ProjectKey)
		var wg sync.WaitGroup
		sem := make(chan struct{}, 5)
		buildKey := cmd.Flag("buildKey").Value.String()
		for _, repo := range repos {
			wg.Add(1)
			go func(r models.RepoEntity) {
				defer wg.Done()
				sem <- struct{}{}
				defer func() { <-sem }()
				svc.Delete(repo.Name, buildKey)
			}(repo)
		}
		wg.Wait()
	},
}

func init() {
	BuildsCmd.Flags().StringVarP(&utils.InputFile, "input", "i", "", "YAML file with list of repositories")
	BuildsCmd.Flags().StringVar(&utils.BuildKey, "buildKey", "", "buildKey of required build")
	BuildsCmd.MarkFlagRequired("buildKey")
}
