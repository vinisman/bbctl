package reviewergroup

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
	"github.com/vinisman/bbctl/internal/bitbucket"
	"github.com/vinisman/bbctl/internal/models"
	"github.com/vinisman/bbctl/utils"
)

// CreateReviewerGroupCmd returns a cobra command to create reviewer groups from a YAML file
func CreateReviewerGroupCmd() *cobra.Command {
	var input string
	var output string

	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create reviewer group from YAML file",
		Long: `Create one or multiple reviewer groups from a YAML file.

Reviewer groups can be used to set default reviewers for pull requests.`,
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

			hasReviewerGroups := false
			for _, repo := range parsed.Repositories {
				if repo.ReviewerGroups != nil && len(*repo.ReviewerGroups) > 0 {
					hasReviewerGroups = true
					break
				}
			}

			if !hasReviewerGroups {
				return fmt.Errorf("no reviewer groups defined in file %s", input)
			}

			updatedRepos, err := client.CreateReviewerGroups(parsed.Repositories)
			if err != nil {
				client.Logger.Error(err.Error())
			}

			if len(updatedRepos) > 0 {
				client.Logger.Info("created reviewer groups", "count", len(updatedRepos))
			}

			return utils.PrintStructured("repositories", updatedRepos, output, "projectKey,repositorySlug,reviewerGroups.id,reviewerGroups.name")
		},
	}

	cmd.Flags().StringVarP(&input, "input", "i", "", `Input YAML or JSON file or '-' for stdin
Example:
  repositories:
    - projectKey: DEV
      repositorySlug: my-repo
      reviewerGroups:
        - name: senior-developers
          description: "Senior developers for code review"
          users:
            - name: john.doe
            - name: jane.smith
`)
	cmd.MarkFlagRequired("input")
	cmd.Flags().StringVarP(&output, "output", "o", "plain", "Output format: plain|yaml|json")

	return cmd
}
