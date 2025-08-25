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
	var file string

	cmd := &cobra.Command{
		Use:   "update",
		Short: "Update webhooks from YAML file by Id",
		RunE: func(cmd *cobra.Command, args []string) error {
			if file == "" {
				return fmt.Errorf("--input is required")
			}

			var parsed models.RepositoryYaml
			if err := utils.ParseYAMLFile(file, &parsed); err != nil {
				return fmt.Errorf("failed to parse YAML file: %w", err)
			}

			if len(parsed.Repositories) == 0 {
				return fmt.Errorf("no repositories found in file %s", file)
			}

			client, err := bitbucket.NewClient(context.Background())
			if err != nil {
				return err
			}

			err = client.UpdateWebhooks(parsed.Repositories)
			if err != nil {
				client.Logger.Error(err.Error())
			}

			return nil
		},
	}

	cmd.Flags().StringVarP(&file, "input", "i", "", `Path to YAML file with webhooks to update
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

	return cmd
}
