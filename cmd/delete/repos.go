package delete

import (
	"context"
	"fmt"
	"os"
	"sync"

	"github.com/spf13/cobra"
	"github.com/vinisman/bbctl/internal"
	"github.com/vinisman/bbctl/utils"
	"gopkg.in/yaml.v3"
)

type RepoYAML struct {
	Name    string `yaml:"name"`
	Slug    string `yaml:"slug"`
	Project string `yaml:"project"`
}

type ReposFile struct {
	Repos []RepoYAML `yaml:"repos"`
}

var (
	reposFile string
)

var deleteReposCmd = &cobra.Command{
	Use:   "repos",
	Short: "Delete multiple repositories from YAML file",
	Long: `Delete multiple Bitbucket repositories from a YAML file.

YAML format:
repos:
  - slug: "my-repo-1"
    project: "PRJ"
  - slug: "my-repo-2"
    project: "PRJ"

Example:
  bbctl delete repos -f repos.yaml -p PROJECT
`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if reposFile == "" {
			return fmt.Errorf("--file is required")
		}

		data, err := os.ReadFile(reposFile)
		if err != nil {
			return fmt.Errorf("failed to read file: %w", err)
		}

		var reposCfg ReposFile
		if err := yaml.Unmarshal(data, &reposCfg); err != nil {
			return fmt.Errorf("failed to parse YAML: %w", err)
		}

		if len(reposCfg.Repos) == 0 {
			return fmt.Errorf("no repositories found in YAML")
		}

		client := internal.NewClient(utils.Cfg.BitbucketURL, utils.Cfg.BitbucketToken, utils.Logger)
		ctx := context.Background()

		workers := utils.WorkerCount
		var wg sync.WaitGroup
		repoCh := make(chan RepoYAML)

		// Workers
		for i := 0; i < workers; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for repo := range repoCh {
					projectKey := repo.Project
					if projectKey == "" {
						projectKey = utils.ProjectKey
					}
					if projectKey == "" {
						utils.Logger.Error("Project key is missing", "repo", repo.Slug)
						continue
					}
					if repo.Slug == "" {
						utils.Logger.Error("Repository slug is missing", "project", projectKey)
						continue
					}

					_, err := client.ProjectAPI.DeleteRepository(ctx, projectKey, repo.Slug).Execute()
					if err != nil {
						utils.Logger.Error("Failed to delete repository", "project", projectKey, "slug", repo.Slug, "error", err)
						continue
					}
					utils.Logger.Info("Repository deleted successfully", "project", projectKey, "slug", repo.Slug)
				}
			}()
		}

		// Send repos to workers
		for _, r := range reposCfg.Repos {
			repoCh <- r
		}
		close(repoCh)

		wg.Wait()
		return nil
	},
}

func init() {
	deleteReposCmd.PersistentFlags().StringVarP(&reposFile, "file", "f", "", "Path to YAML file with repositories to delete")
	deleteReposCmd.PersistentFlags().StringVarP(&utils.ProjectKey, "project", "p", "", "Default Bitbucket project key (overridden by repo.project from file)(Optional)")
}
