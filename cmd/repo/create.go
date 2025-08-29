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

func NewCreateCmd() *cobra.Command {
	var (
		projectKey     string
		name           string
		repositorySlug string
		description    string
		defaultBranch  string
		file           string
	)

	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create a repository",
		Long: `Create a repository in a given Bitbucket project.
You must specify either:
  --projectKey and --name (to create a single repository),
  --input (YAML file with one or more repositories to create).`,
		RunE: func(cmd *cobra.Command, args []string) error {
			// Validate arguments
			if (file != "" && (projectKey != "" || name != "" || repositorySlug != "")) || (file == "" && (projectKey == "" || name == "")) {
				return fmt.Errorf("either --input or (--projectKey and --name) must be specified")
			}

			client, err := bitbucket.NewClient(context.Background())
			if err != nil {
				return err
			}

			var repos []models.ExtendedRepository

			// Case 1: create from YAML file
			if file != "" {
				var parsed struct {
					Repositories []models.ExtendedRepository `yaml:"repositories"`
				}
				if err := utils.ParseYAMLFile(file, &parsed); err != nil {
					return fmt.Errorf("failed to parse YAML file %s: %w", file, err)
				}
				if len(parsed.Repositories) == 0 {
					return fmt.Errorf("no repositories found in %s", file)
				}
				repos = parsed.Repositories
			} else {
				// Case 2: create a single repository from CLI flags
				repo := models.ExtendedRepository{
					ProjectKey:     projectKey,
					RepositorySlug: repositorySlug,
					RestRepository: openapi.RestRepository{
						Name:          utils.OptionalString(name),
						Description:   utils.OptionalString(description),
						ScmId:         utils.OptionalString("git"),
						Forkable:      utils.OptionalBool(true),
						DefaultBranch: utils.OptionalString(defaultBranch),
					},
				}
				repos = []models.ExtendedRepository{repo}
			}

			err = client.CreateRepos(repos)
			if err != nil {
				client.Logger.Error(err.Error())
			}
			return nil
		},
	}

	cmd.Flags().StringVarP(&projectKey, "projectKey", "k", "", "Project key (required if --input not used)")
	cmd.Flags().StringVar(&name, "name", "", "Repository name (required if --input not used)")
	cmd.Flags().StringVarP(&repositorySlug, "repositorySlug", "s", "", "Slug of the repository (optional, required if --input not used)")
	cmd.Flags().StringVar(&description, "desc", "", "Repository description (optional)")
	cmd.Flags().StringVar(&defaultBranch, "default-branch", "", "Default branch name (optional)")
	cmd.Flags().StringVarP(&file, "input", "i", "", `Path to YAML file with repositories to create.
Example YAML:
repositories:
  - projectKey: PRJ1
    repository:
      name: REPO1
      description: Description
      defaultBranch: master
`)

	return cmd
}
