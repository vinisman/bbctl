package repo

import (
	"context"
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"github.com/vinisman/bbctl/internal/bitbucket"
	"github.com/vinisman/bbctl/internal/models"
	"github.com/vinisman/bbctl/utils"
)

func NewDeleteCmd() *cobra.Command {
	var (
		repositorySlug string
		file           string
	)

	cmd := &cobra.Command{
		Use:   "delete",
		Short: "Delete a repository",
		Long:  `Delete one or more repositories. You must specify either --repositorySlug in the format <projectKey>/<repositorySlug> for a single repository, or --input for a YAML file with multiple repositories.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			// Validate input: either single repo or YAML file
			if (file != "" && repositorySlug != "") || (file == "" && repositorySlug == "") {
				return fmt.Errorf("either --repositorySlug or --input must be specified, but not both")
			}

			client, err := bitbucket.NewClient(context.Background())
			if err != nil {
				return err
			}

			// Case 1: delete from YAML file
			if file != "" {
				var parsed models.RepositoryYaml

				if err := utils.ParseYAMLFile(file, &parsed); err != nil {
					return err
				}
				if len(parsed.Repositories) == 0 {
					return fmt.Errorf("no repositories found in %s", file)
				}
				return client.DeleteRepos(parsed.Repositories)
			}

			// Case 2: delete single repo by project+slug
			parts := strings.SplitN(repositorySlug, "/", 2)
			if len(parts) != 2 || parts[1] == "" {
				client.Logger.Error("invalid repository identifier format, repository slug is empty")
				return fmt.Errorf("invalid repository identifier format, expected <projectKey>/<repositorySlug>")
			}
			projectKey := parts[0]
			slug := parts[1]

			ref := models.ExtendedRepository{
				ProjectKey:     projectKey,
				RepositorySlug: slug,
			}
			err = client.DeleteRepos([]models.ExtendedRepository{ref})
			if err != nil {
				client.Logger.Error(err.Error())
			}
			return nil
		},
	}

	cmd.Flags().StringVarP(&repositorySlug, "repositorySlug", "s", "", "Identifier of the current repository in format <projectKey>/<repositorySlug> (required if --input not used)")
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
