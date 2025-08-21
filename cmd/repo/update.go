package repo

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
	"github.com/vinisman/bbctl/internal/bitbucket"
	"github.com/vinisman/bbctl/internal/models"
	"github.com/vinisman/bbctl/utils"
	openapi "github.com/vinisman/bitbucket-sdk-go/openapi"
)

func NewUpdateCmd() *cobra.Command {
	var (
		projectKey     string
		repositorySlug string
		name           string
		description    string
		defaultBranch  string
		file           string
	)

	cmd := &cobra.Command{
		Use:   "update",
		Short: "Update a repository",
		Long: `Update a single repository or multiple repositories defined in a YAML file.
You must specify either --projectKey and --repositorySlug for a single repository, 
or --input for a YAML file containing multiple repositories.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			// Validate input: either single repo or file
			if (file != "" && (projectKey != "" || repositorySlug != "")) ||
				(file == "" && (projectKey == "" || repositorySlug == "")) {
				return fmt.Errorf("either --input or (--projectKey and --repositorySlug) must be specified, but not both")
			}

			client, err := bitbucket.NewClient(context.Background())
			if err != nil {
				return err
			}

			// Case 1: update from YAML file
			if file != "" {
				var parsed struct {
					Repositories []models.ExtendedRepository `yaml:"repositories"`
				}
				if err := utils.ParseYAMLFile(file, &parsed); err != nil {
					return err
				}
				return client.UpdateRepos(parsed.Repositories)
			}

			// Case 2: update single repository from CLI flags
			repo := models.ExtendedRepository{
				ProjectKey:     projectKey,
				RepositorySlug: repositorySlug,
				RestRepository: openapi.RestRepository{
					Slug:        &repositorySlug,
					Name:        utils.OptionalString(name),
					Description: utils.OptionalString(description),
				},
			}

			if defaultBranch != "" {
				repo.RestRepository.DefaultBranch = &defaultBranch
			}

			return client.UpdateRepos([]models.ExtendedRepository{repo})
		},
	}

	cmd.Flags().StringVarP(&projectKey, "projectKey", "k", "", "Project key (required if --input not used)")
	cmd.Flags().StringVarP(&repositorySlug, "repositorySlug", "s", "", "Repository slug (required if --input not used)")
	cmd.Flags().StringVar(&name, "name", "", "Repository name (optional)")
	cmd.Flags().StringVar(&description, "desc", "", "Repository description (optional)")
	cmd.Flags().StringVar(&defaultBranch, "default-branch", "", "Default branch name (optional)")
	cmd.Flags().StringVarP(&file, "input", "i", "", `Path to YAML file with repositories to update.
If specified, multiple repositories can be updated in a single operation.
The file must contain a list of repositories with at least the "projectKey" and "repositorySlug" fields.
Other fields are optional and will be updated only if provided.

Example YAML:
repositories:
  - projectKey: DEV
    repositorySlug: my-repo
    repository:
      name: "New Repo Name"
      description: "Updated description"
      defaultBranch: "develop"
`)

	return cmd
}
