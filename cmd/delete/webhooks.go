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

type RepoWebhookYAML struct {
	Project  string `yaml:"project"`
	RepoSlug string `yaml:"slug"`
	Name     string `yaml:"name"`
}

type RepoWebhooksFile struct {
	Webhooks []RepoWebhookYAML `yaml:"webhooks"`
}

var (
	webhooksFile string
)

var deleteRepoWebhooksCmd = &cobra.Command{
	Use:   "repo-webhooks",
	Short: "Delete multiple repository webhooks from a YAML file",
	Long: `Delete multiple repository webhooks by name from a YAML file.

YAML format example:

webhooks:
  - project: PRJ
    slug: my-repo
    name: "CI Hook"
  - project: PRJ
    slug: another-repo
    name: "Deploy Hook"

Example usage:

  bbctl delete repo-webhooks -f webhooks.yaml
`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if webhooksFile == "" {
			return fmt.Errorf("--file is required")
		}

		data, err := os.ReadFile(webhooksFile)
		if err != nil {
			return fmt.Errorf("failed to read file: %w", err)
		}

		var whCfg RepoWebhooksFile
		if err := yaml.Unmarshal(data, &whCfg); err != nil {
			return fmt.Errorf("failed to parse YAML: %w", err)
		}

		if len(whCfg.Webhooks) == 0 {
			return fmt.Errorf("no webhooks found in YAML")
		}

		client := internal.NewClient(utils.Cfg.BitbucketURL, utils.Cfg.BitbucketToken, utils.Logger)
		ctx := context.Background()

		workers := utils.WorkerCount
		var wg sync.WaitGroup
		ch := make(chan RepoWebhookYAML)

		for i := 0; i < workers; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for wh := range ch {
					projectKey := wh.Project
					if projectKey == "" {
						projectKey = utils.ProjectKey
					}
					if projectKey == "" {
						utils.Logger.Error("Project key missing for webhook", "name", wh.Name, "repo", wh.RepoSlug)
						continue
					}
					if wh.RepoSlug == "" {
						utils.Logger.Error("Repository slug missing for webhook", "name", wh.Name)
						continue
					}
					if wh.Name == "" {
						utils.Logger.Error("Webhook name missing for repo", "repo", wh.RepoSlug)
						continue
					}

					webhookID, err := internal.FindWebhookIDByName(ctx, client, projectKey, wh.RepoSlug, wh.Name)
					if err != nil {
						utils.Logger.Error("Failed to find webhook", "repo", wh.RepoSlug, "name", wh.Name, "error", err)
						continue
					}
					if webhookID == "" {
						utils.Logger.Info("Webhook not found, skipping delete", "name", wh.Name, "repo", wh.RepoSlug)
						continue
					}

					httpResp, err := client.RepositoryAPI.DeleteWebhook1(ctx, projectKey, webhookID, wh.RepoSlug).Execute()
					if err != nil {
						utils.Logger.Error("Failed to delete repository webhook", "repo", wh.RepoSlug, "name", wh.Name, "error", err)
						utils.Logger.Debug("Details", "httpResp", httpResp.Body)
						continue
					}
					utils.Logger.Info("Repository webhook deleted successfully", "project", projectKey, "repo", wh.RepoSlug, "name", wh.Name)
				}
			}()
		}

		for _, wh := range whCfg.Webhooks {
			ch <- wh
		}
		close(ch)
		wg.Wait()

		return nil
	},
}

func init() {
	deleteRepoWebhooksCmd.PersistentFlags().StringVarP(&webhooksFile, "file", "f", "", "YAML file with repository webhooks")
}
