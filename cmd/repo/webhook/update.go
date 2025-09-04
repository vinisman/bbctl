package webhook

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
	"github.com/vinisman/bbctl/internal/bitbucket"
	"github.com/vinisman/bbctl/internal/models"
	"github.com/vinisman/bbctl/utils"
)

// UpdateWebHookCmd returns a cobra command to update webhooks from a YAML file
func UpdateWebHookCmd() *cobra.Command {
	var input string
	var output string

	cmd := &cobra.Command{
		Use:   "update",
		Short: "Update webhooks from YAML file by Id",
		RunE: func(cmd *cobra.Command, args []string) error {
			if input == "" {
				return fmt.Errorf("--input is required")
			}

			var parsed models.RepositoryYaml
			if err := utils.ParseFile(input, &parsed); err != nil {
				return fmt.Errorf("failed to parse file: %w", err)
			}

			if len(parsed.Repositories) == 0 {
				client, _ := bitbucket.NewClient(context.Background())
				if client != nil {
					client.Logger.Info("no repositories found in file", "file", input)
				}
				return nil
			}

			client, err := bitbucket.NewClient(context.Background())
			if err != nil {
				return err
			}

			updated, err := client.UpdateWebhooks(parsed.Repositories)
			if err != nil {
				client.Logger.Error(err.Error())
			}
			if len(updated) > 0 {
				for _, r := range updated {
					for _, wh := range *r.Webhooks {
						client.Logger.Info("Updated webhook",
							"project", r.ProjectKey,
							"repo", r.RepositorySlug,
							"id", utils.Int32PtrToString(wh.Id),
							"name", utils.SafeValue(wh.Name))
					}
				}
			}

			if output != "yaml" && output != "json" {
				return fmt.Errorf("invalid output format: %s, allowed values: yaml, json", output)
			}

			return utils.PrintStructured("repositories", updated, output, "")
		},
	}

	cmd.Flags().StringVarP(&input, "input", "i", "", `Path to YAML or JSON file with webhooks to update or '-' to read from stdin
Example:
  repositories:
    - projectKey: DEV
      repositorySlug: repo1
      webhooks:
        - id: 123
          name: build-hook
          url: https://ci.example.com/webhook
          events:
            - repo:refs_changed
          active: true
          scopeType: REPOSITORY
          sslVerificationRequired: true
`)

	cmd.Flags().StringVarP(&output, "output", "o", "", "Optional path to save updated webhooks as YAML/JSON")

	return cmd
}
