package create

import (
	"context"
	"fmt"
	"os"
	"sync"

	"github.com/spf13/cobra"
	"github.com/vinisman/bbctl/internal"
	"github.com/vinisman/bbctl/utils"
	openapi "github.com/vinisman/bitbucket-sdk-go/openapi"
	"gopkg.in/yaml.v3"
)

type RepoYAML struct {
	Name        string `yaml:"name"`
	Slug        string `yaml:"slug"`
	Branch      string `yaml:"branch"`
	Project     string `yaml:"project"`
	Description string `yaml:"description"`
}

type ReposFile struct {
	Repos []RepoYAML `yaml:"repos"`
}

var (
	reposFile string
)

var createReposCmd = &cobra.Command{
	Use:   "repos",
	Short: "Create multiple repositories from YAML file",
	Long: `Create multiple Bitbucket repositories from a YAML file.

YAML format:
repos:
  - name: "My Repo 1"
    slug: "my-repo-1"
    branch: "main"
    project: "PRJ"
    description: "My first repo"
  - name: "My Repo 2"
    slug: "my-repo-2"
    branch: "develop"
    project: "PRJ"

Example:
  bbctl create repos -f repos.yaml -p PROJECT
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
						utils.Logger.Error("Project key is missing", "repo", repo.Name)
						continue
					}

					scmId := "git"
					req := openapi.RestRepository{
						Name:          &repo.Name,
						Slug:          &repo.Slug,
						ScmId:         &scmId,
						DefaultBranch: &repo.Branch,
					}

					if repo.Description != "" {
						req.Description = &repo.Description
					}

					_, _, err := client.ProjectAPI.CreateRepository(ctx, projectKey).RestRepository(req).Execute()
					if err != nil {
						utils.Logger.Error("Failed to create repository", "repo", repo.Name, "error", err)
						continue
					}
					utils.Logger.Info("Repository created successfully", "name", repo.Name, "slug", repo.Slug)
				}
			}()
		}

		// Send repos to workers
		for _, r := range reposCfg.Repos {
			if r.Branch == "" {
				r.Branch = "main"
			}
			repoCh <- r
		}
		close(repoCh)

		wg.Wait()
		return nil
	},
}

func init() {
	createReposCmd.PersistentFlags().StringVarP(&reposFile, "file", "f", "", "Path to YAML file with repositories")
	createReposCmd.PersistentFlags().StringVarP(&utils.ProjectKey, "project", "p", "", "Default Bitbucket project key (overridden by repo.project from file)(Optional)")
}
