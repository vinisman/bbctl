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

type RepoWebhookYAML struct {
	Project         string   `yaml:"project"`
	RepoSlug        string   `yaml:"repoSlug"`
	Name            string   `yaml:"name"`
	URL             string   `yaml:"url"`
	Events          []string `yaml:"events"`
	Active          *bool    `yaml:"active,omitempty"`
	Username        string   `yaml:"username,omitempty"`
	Password        string   `yaml:"password,omitempty"`
	SSLVerification *bool    `yaml:"sslVerificationRequired,omitempty"`
}

type RepoWebhooksFile struct {
	Webhooks []RepoWebhookYAML `yaml:"webhooks"`
}

var (
	webhooksFile           string
	globalWebhookName      string
	globalWebhookURL       string
	globalWebhookEvents    []string
	globalWebhookActive    bool = true
	globalWebhookUsername  string
	globalWebhookPassword  string
	globalWebhookSSLVerify bool = true
)

var createRepoWebhooksCmd = &cobra.Command{
	Use:   "repo-webhooks",
	Short: "Create multiple repository webhooks from a YAML file",
	Long: `Create multiple repository webhooks from a YAML file.

YAML format example:

webhooks:
  - project: PRJ
    repoSlug: my-repo
    name: "CI Hook"
    url: https://ci.example.com/hook
    events:
      - repo:refs_changed
      - pr:opened
    active: true
    username: user
    password: pass
    sslVerificationRequired: false

Example usage:

  bbctl create repo-webhooks -f webhooks.yaml
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

					// Подставляем name из глобального параметра, если пустое
					name := wh.Name
					if name == "" {
						name = globalWebhookName
					}
					if name == "" {
						utils.Logger.Error("Webhook name missing", "repo", wh.RepoSlug)
						continue
					}

					// URL
					url := wh.URL
					if url == "" {
						url = globalWebhookURL
					}
					if url == "" {
						utils.Logger.Error("Webhook URL missing", "name", name, "repo", wh.RepoSlug)
						continue
					}

					// Events
					events := wh.Events
					if len(events) == 0 {
						events = globalWebhookEvents
					}
					if len(events) == 0 {
						utils.Logger.Error("Webhook events missing", "name", name, "repo", wh.RepoSlug)
						continue
					}

					// Active
					active := globalWebhookActive
					if wh.Active != nil {
						active = *wh.Active
					}

					// SSL Verification
					sslVerify := globalWebhookSSLVerify
					if wh.SSLVerification != nil {
						sslVerify = *wh.SSLVerification
					}

					// Credentials
					username := wh.Username
					if username == "" {
						username = globalWebhookUsername
					}
					password := wh.Password
					if password == "" {
						password = globalWebhookPassword
					}

					scopeType := "repository"
					req := openapi.RestWebhook{
						Name:                    &name,
						Url:                     &url,
						Events:                  events,
						Active:                  openapi.PtrBool(active),
						SslVerificationRequired: openapi.PtrBool(sslVerify),
						ScopeType:               &scopeType,
					}

					if username != "" || password != "" {
						req.Credentials = &openapi.RestWebhookCredentials{
							Username: &username,
							Password: &password,
						}
					}

					_, _, err := client.RepositoryAPI.CreateWebhook1(ctx, projectKey, wh.RepoSlug).
						RestWebhook(req).
						Execute()
					if err != nil {
						utils.Logger.Error("Failed to create repository webhook", "repo", wh.RepoSlug, "name", name, "error", err)
						continue
					}
					utils.Logger.Info("Repository webhook created", "project", projectKey, "repo", wh.RepoSlug, "name", name)
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
	createRepoWebhooksCmd.PersistentFlags().StringVarP(&webhooksFile, "file", "f", "", "YAML file with repository webhooks")

	createRepoWebhooksCmd.PersistentFlags().StringVar(&globalWebhookName, "name", "", "Global webhook display name (used if missing in YAML)")
	createRepoWebhooksCmd.PersistentFlags().StringVar(&globalWebhookURL, "url", "", "Global webhook target URL (used if missing in YAML)")
	createRepoWebhooksCmd.PersistentFlags().StringSliceVar(&globalWebhookEvents, "events", nil, "Global comma-separated list of webhook events (used if missing in YAML)")
	createRepoWebhooksCmd.PersistentFlags().BoolVar(&globalWebhookActive, "active", true, "Global active state for webhook (used if missing in YAML)")
	createRepoWebhooksCmd.PersistentFlags().StringVar(&globalWebhookUsername, "username", "", "Global HTTP basic auth username (used if missing in YAML)")
	createRepoWebhooksCmd.PersistentFlags().StringVar(&globalWebhookPassword, "password", "", "Global HTTP basic auth password (used if missing in YAML)")
	createRepoWebhooksCmd.PersistentFlags().BoolVar(&globalWebhookSSLVerify, "ssl-verification", true, "Global SSL verification flag (used if missing in YAML)")
}
