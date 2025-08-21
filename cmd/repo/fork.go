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

func NewForkCmd() *cobra.Command {
	var (
		projectKey        string
		repositorySlug    string
		forkProject       string
		forkName          string
		forkDefaultBranch string
		forkDescription   string
		file              string
	)

	cmd := &cobra.Command{
		Use:   "fork",
		Short: "Fork a repository",
		Long: `Fork a single repository or multiple repositories defined in a YAML file.
You must specify either --projectKey and --repositorySlug with --fProjectKey for a single repository,
or --input for a YAML file containing multiple forks.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			// Validate input: either single fork or file
			if (file != "" && (projectKey != "" || repositorySlug != "" || forkProject != "")) ||
				(file == "" && (projectKey == "" || repositorySlug == "" || forkProject == "")) {
				return fmt.Errorf("either --input, or --projectKey/--repositorySlug with --fProjectKey must be specified, but not both")
			}

			client, err := bitbucket.NewClient(context.Background())
			if err != nil {
				return err
			}

			// Case 1: fork from YAML file
			if file != "" {
				var parsed struct {
					Repositories []models.ExtendedRepository `yaml:"repositories"`
				}
				if err := utils.ParseYAMLFile(file, &parsed); err != nil {
					return fmt.Errorf("failed to parse YAML file: %w", err)
				}
				return client.ForkRepos(parsed.Repositories)
			}

			// Case 2: fork single repository
			repo := models.ExtendedRepository{
				ProjectKey:     projectKey,
				RepositorySlug: repositorySlug,
				RestRepository: openapi.RestRepository{
					Name:        &forkName,
					Description: &forkDescription,
					ScmId:       utils.OptionalString("git"),
					Forkable:    utils.OptionalBool(true),
					DefaultBranch: func() *string {
						if forkDefaultBranch == "" {
							return nil
						}
						return &forkDefaultBranch
					}(),
					Project: &openapi.RestChangesetRepositoryOriginProject{
						Key: forkProject,
					},
				},
			}

			err = client.ForkRepos([]models.ExtendedRepository{repo})
			if err != nil {
				return err
			}

			fmt.Printf("Forked repository: %s/%s -> %s\n", projectKey, repositorySlug, forkProject)
			return nil
		},
	}

	cmd.Flags().StringVarP(&projectKey, "projectKey", "k", "", "Project key of the repository to fork (required if --input not used)")
	cmd.Flags().StringVarP(&repositorySlug, "repositorySlug", "s", "", "Slug of the repository to fork (required if --input not used)")
	cmd.Flags().StringVar(&forkProject, "fProjectKey", "", "Project key where the fork will be created (required if --input not used)")
	cmd.Flags().StringVar(&forkName, "name", "", "Optional name of the forked repository")
	cmd.Flags().StringVar(&forkDefaultBranch, "defaultBranch", "", "Optional default branch for the forked repository")
	cmd.Flags().StringVar(&forkDescription, "description", "", "Optional description for the forked repository")
	cmd.Flags().StringVarP(&file, "input", "i", "", `Path to YAML file with forks to create.
Example YAML structure:
repositories:
  - projectKey: PRJ1
    repositorySlug: repo1
    repository:
      name: REPO1
      description: Description
      defaultBranch: master
      project:
        key: PRJ2
`)

	return cmd
}
