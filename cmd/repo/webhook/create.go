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
			if err := utils.ParseFile(input, &parsed); err != nil {
				return fmt.Errorf("failed to parse input file: %w", err)
			}

			client, err := bitbucket.NewClient(context.Background())
			if err != nil {
				return err
			}

			if len(parsed.Repositories) == 0 {
				client.Logger.Info("no repositories found in file", "file", input)
				return nil
			}

			hasWebhooks := false
			for _, repo := range parsed.Repositories {
				if repo.Webhooks != nil && len(*repo.Webhooks) > 0 {
					hasWebhooks = true
					break
				}
			}

			if !hasWebhooks {
				return fmt.Errorf("no webhooks defined in file %s", input)
			}

			updatedRepos, err := client.CreateWebhooks(parsed.Repositories)
			if err != nil {
				client.Logger.Error(err.Error())
			}

			// Only print output if output format is specified
			if output != "" {
				if output != "yaml" && output != "json" {
					return fmt.Errorf("invalid output format: %s, allowed values: yaml, json", output)
				}
				return utils.PrintStructured("repositories", updatedRepos, output, "")
			}

			return nil
		},
	}

	cmd.Flags().StringVarP(&input, "input", "i", "", `Path to YAML or JSON file or '-' to read from stdin
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
	cmd.Flags().StringVarP(&output, "output", "o", "", "Optional output format: yaml or json")

	return cmd
}
