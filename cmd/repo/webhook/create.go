package webhook

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
	"github.com/vinisman/bbctl/internal/bitbucket"
	"github.com/vinisman/bbctl/internal/models"
	"github.com/vinisman/bbctl/utils"
)

// CreateWebHookCmd returns a cobra command to create webhooks from a YAML file
func CreateWebHookCmd() *cobra.Command {
	var input string
	var output string

	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create webhook from YAML file",
		Long: `Create one or multiple webhooks from a YAML file.

Be careful: Bitbucket allows webhooks with duplicate names, 
so make sure to use unique names to avoid confusion or accidental overwrites.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if input == "" {
				return fmt.Errorf("--input is required")
			}

			var parsed models.RepositoryYaml
			if err := utils.ParseYAMLFile(input, &parsed); err != nil {
				return fmt.Errorf("failed to parse YAML file: %w", err)
			}

			if len(parsed.Repositories) == 0 {
				return fmt.Errorf("no repositories found in file %s", input)
			}

			hasWebhooks := false
			for _, repo := range parsed.Repositories {
				if len(repo.Webhooks) > 0 {
					hasWebhooks = true
					break
				}
			}
			if !hasWebhooks {
				return fmt.Errorf("no webhooks defined in file %s", input)
			}

			client, err := bitbucket.NewClient(context.Background())
			if err != nil {
				return err
			}

			updatedRepos, err := client.CreateWebhooks(parsed.Repositories)
			if err != nil {
				client.Logger.Error(err.Error())
			}

			if output == "" {
				// Print info logs for each created webhook when output is empty
				for _, repo := range updatedRepos {
					for _, wh := range repo.Webhooks {
						client.Logger.Info(fmt.Sprintf("Created webhook %s for repository %s/%s", utils.Int32PtrToString(wh.Id), repo.ProjectKey, repo.RepositorySlug))
					}
				}
				return nil
			}

			if output != "yaml" && output != "json" {
				return fmt.Errorf("invalid output format: %s, allowed values: yaml, json", output)
			}

			parsed.Repositories = updatedRepos
			return utils.PrintStructured("repositories", parsed.Repositories, output, "")
		},
	}

	cmd.Flags().StringVarP(&input, "input", "i", "", `Path to YAML file or '-' to read from stdin
Example:
repositories:
  - projectKey: DEV
    repositorySlug: my-repo
    webhooks:
      - name: build-hook
        url: https://ci.example.com/webhook
        events:
          - repo:refs_changed
        active: true
        scopeType: REPOSITORY
        sslVerificationRequired: true
        configuration:
          key1: value1
        credentials:
          username: myuser
          password: mypass
`)
	cmd.Flags().StringVarP(&output, "output", "o", "yaml", "Output format: yaml or json (default: yaml)")

	return cmd
}
