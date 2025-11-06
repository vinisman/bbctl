package reviewergroup

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
	"github.com/vinisman/bbctl/internal/bitbucket"
	"github.com/vinisman/bbctl/internal/models"
	"github.com/vinisman/bbctl/utils"
)

// UpdateReviewerGroupCmd returns a cobra command to update reviewer groups from a YAML file
func UpdateReviewerGroupCmd() *cobra.Command {
	var input string
	var output string

	cmd := &cobra.Command{
		Use:   "update",
		Short: "Update reviewer groups from YAML file by Id",
		Long: `Update existing reviewer groups from a YAML file.
		
Each reviewer group must have an ID field to identify which group to update.`,
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

			updated, err := client.UpdateReviewerGroups(parsed.Repositories)
			if err != nil {
				client.Logger.Error(err.Error())
			}

			// Only print output if output format is specified
			if output != "" {
				if output != "yaml" && output != "json" {
					return fmt.Errorf("invalid output format: %s, allowed values: yaml, json", output)
				}
				return utils.PrintStructured("repositories", updated, output, "")
			}

			return nil
		},
	}

	cmd.Flags().StringVarP(&input, "input", "i", "", `Path to YAML or JSON file with reviewer groups to update or '-' to read from stdin
Example:
  repositories:
    - projectKey: DEV
      repositorySlug: repo1
      reviewerGroups:
        - id: 123
          name: senior-developers
          description: "Senior developers group for code reviews"
          users:
            - name: john.doe
            - name: jane.smith
`)

	cmd.Flags().StringVarP(&output, "output", "o", "", "Optional path to save updated reviewer groups as YAML/JSON")

	return cmd
}
