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
		input          string
		output         string
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
			if (input != "" && (projectKey != "" || name != "" || repositorySlug != "")) || (input == "" && (projectKey == "" || name == "")) {
				return fmt.Errorf("either --input or (--projectKey and --name) must be specified")
			}

			client, err := bitbucket.NewClient(context.Background())
			if err != nil {
				return err
			}

			var repos []models.ExtendedRepository

			// Case 1: create from YAML file
			if input != "" {
				var parsed struct {
					Repositories []models.ExtendedRepository `yaml:"repositories"`
				}
				if err := utils.ParseFile(input, &parsed); err != nil {
					return fmt.Errorf("failed to parse file %s: %w", input, err)
				}
				if len(parsed.Repositories) == 0 {
					return fmt.Errorf("no repositories found in %s", input)
				}
				repos = parsed.Repositories
			} else {
				// Case 2: create a single repository from CLI flags
				repo := models.ExtendedRepository{
					ProjectKey:     projectKey,
					RepositorySlug: repositorySlug,
					RestRepository: &openapi.RestRepository{
						Name:          utils.OptionalString(name),
						Description:   utils.OptionalString(description),
						ScmId:         utils.OptionalString("git"),
						Forkable:      utils.OptionalBool(true),
						DefaultBranch: utils.OptionalString(defaultBranch),
					},
				}
				repos = []models.ExtendedRepository{repo}
			}

			createdRepos, err := client.CreateRepos(repos)
			if err != nil {
				client.Logger.Error(err.Error())
			}

			// Only print output if output format is specified
			if output != "" {
				if output != "yaml" && output != "json" {
					return fmt.Errorf("invalid output format: %s, allowed values: yaml, json", output)
				}
				return utils.PrintStructured("repositories", createdRepos, output, "")
			}
			return nil
		},
	}

	cmd.Flags().StringVarP(&projectKey, "projectKey", "k", "", "Project key (required if --input not used)")
	cmd.Flags().StringVar(&name, "name", "", "Repository name (required if --input not used)")
	cmd.Flags().StringVarP(&repositorySlug, "repositorySlug", "s", "", "Slug of the repository (optional, required if --input not used)")
	cmd.Flags().StringVar(&description, "desc", "", "Repository description (optional)")
	cmd.Flags().StringVar(&defaultBranch, "default-branch", "", "Default branch name (optional)")
	cmd.Flags().StringVarP(&input, "input", "i", "", `Path to YAML or JSON file with repositories to create. Use '-' to read from stdin.
Example YAML:
repositories:
  - projectKey: PRJ1
    restRepository:
      name: REPO1
      description: Description
      defaultBranch: master
`)
	cmd.Flags().StringVarP(&output, "output", "o", "", "Optional output format: yaml or json")

	return cmd
}
