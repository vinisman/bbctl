package requiredbuild

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
	"github.com/vinisman/bbctl/internal/bitbucket"
	"github.com/vinisman/bbctl/internal/models"
	"github.com/vinisman/bbctl/utils"
)

func UpdateRequiredBuildCmd() *cobra.Command {
	var input string

	cmd := &cobra.Command{
		Use:   "update",
		Short: "Update required-builds from YAML file by Id",
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

			err = client.UpdateRequiredBuilds(parsed.Repositories)
			if err != nil {
				client.Logger.Error(err.Error())
			}
			return nil
		},
	}

	cmd.Flags().StringVarP(&input, "input", "i", "", `Path to YAML or JSON file with webhooks to update. Use '-' to read from stdin.
Example:
repositories:
  - projectKey: project_1
    repositorySlug: repo1
    requiredBuilds:  
      - buildparentkeys:
          - job1
          - job2
		id: 1
        exemptrefmatcher: null
        refmatcher:
          displayid: ANY_REF_MATCHER_ID
          id: ANY_REF_MATCHER_ID
          type:
              id: ANY_REF
              name: Any branch
`)

	return cmd
}
