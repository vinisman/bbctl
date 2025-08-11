package apply

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
	applyFile string
)

var applyReposCmd = &cobra.Command{
	Use:   "repos",
	Short: "Create or update repositories from YAML file",
	Long: `Create or update Bitbucket repositories from a YAML file.

If a repository exists (matched by slug), it will be updated.
If it does not exist, it will be created.

YAML format:
repos:
  - name: "Repo 1"
    slug: "repo-1"
    branch: "main"
    project: "PRJ"
    description: "My first repo"
  - name: "Repo 2"
    slug: "repo-2"
    branch: "develop"
    project: "PRJ"
`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if applyFile == "" {
			return fmt.Errorf("--file is required")
		}

		data, err := os.ReadFile(applyFile)
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
					if repo.Slug == "" {
						utils.Logger.Error("Slug is missing", "repo", repo.Name)
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

					// Check if exists
					_, resp, err := client.ProjectAPI.GetRepository(ctx, projectKey, repo.Slug).Execute()
					if err == nil && resp.StatusCode == 200 {
						// Exists -> Update
						_, _, err = client.ProjectAPI.UpdateRepository(ctx, projectKey, repo.Slug).RestRepository(req).Execute()
						if err != nil {
							utils.Logger.Error("Failed to update repository", "repo", repo.Name, "error", err)
							continue
						}
						utils.Logger.Info("Repository updated successfully", "name", repo.Name, "slug", repo.Slug)
					} else {
						// Not found -> Create
						_, _, err = client.ProjectAPI.CreateRepository(ctx, projectKey).RestRepository(req).Execute()
						if err != nil {
							utils.Logger.Error("Failed to create repository", "repo", repo.Name, "error", err)
							continue
						}
						utils.Logger.Info("Repository created successfully", "name", repo.Name, "slug", repo.Slug)
					}
				}
			}()
		}

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
	applyReposCmd.PersistentFlags().StringVarP(&applyFile, "file", "f", "", "Path to YAML file with repositories")
	applyReposCmd.PersistentFlags().StringVarP(&utils.ProjectKey, "project", "p", "", "Default Bitbucket project key (overridden by repo.project)")
}
