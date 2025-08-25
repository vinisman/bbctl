package repo

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
	"github.com/vinisman/bbctl/internal/bitbucket"
	"github.com/vinisman/bbctl/internal/models"
	"github.com/vinisman/bbctl/utils"
)

func NewDeleteCmd() *cobra.Command {
	var (
		projectKey     string
		repositorySlug string
		file           string
	)

	cmd := &cobra.Command{
		Use:   "delete",
		Short: "Delete a repository",
		Long:  `Delete one or more repositories. You must specify either --project and --slug for a single repository, or --input for a YAML file with multiple repositories.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			// Validate input: either single repo or YAML file
			if (file != "" && (projectKey != "" || repositorySlug != "")) ||
				(file == "" && (projectKey == "" || repositorySlug == "")) {
				return fmt.Errorf("either --project and --slug, or --input must be specified, but not both")
			}

			client, err := bitbucket.NewClient(context.Background())
			if err != nil {
				return err
			}

			// Case 1: delete from YAML file
			if file != "" {
				var parsed struct {
					Repositories []models.ExtendedRepository `yaml:"repositories"`
				}
				if err := utils.ParseYAMLFile(file, &parsed); err != nil {
					return err
				}
				if len(parsed.Repositories) == 0 {
					return fmt.Errorf("no repositories found in %s", file)
				}
				return client.DeleteRepos(parsed.Repositories)
			}

			// Case 2: delete single repo by project+slug
			ref := models.ExtendedRepository{
				ProjectKey:     projectKey,
				RepositorySlug: repositorySlug,
			}
			err = client.DeleteRepos([]models.ExtendedRepository{ref})
			if err != nil {
				client.Logger.Error(err.Error())
			}
			return nil
		},
	}

	cmd.Flags().StringVarP(&projectKey, "project", "k", "", "Project key of the repository (required if --input not used)")
	cmd.Flags().StringVarP(&repositorySlug, "repositorySlug", "s", "", "Slug of the repository (required if --input not used)")
	cmd.Flags().StringVarP(&file, "input", "i", "", `Path to YAML file with repositories to delete.
Example file content:
repositories:
  - projectKey: PRJ1
    repositorySlug: repo1
  - projectKey: PRJ2
    repositorySlug: repo2
`)

	return cmd
}
