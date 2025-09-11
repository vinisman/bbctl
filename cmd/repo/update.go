package repo

import (
	"context"
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"github.com/vinisman/bbctl/internal/bitbucket"
	"github.com/vinisman/bbctl/internal/models"
	"github.com/vinisman/bbctl/utils"
	openapi "github.com/vinisman/bitbucket-sdk-go/openapi"
)

func NewUpdateCmd() *cobra.Command {
	var (
		repositorySlug string
		name           string
		description    string
		defaultBranch  string
		input          string
		newProjectKey  string
		output         string
	)

	cmd := &cobra.Command{
		Use:   "update",
		Short: "Update a repository",
		Long: `Update a single repository defined by --repositorySlug in format <projectKey>/<repositorySlug>, 
or multiple repositories defined in a YAML file with --input.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			// Validate input: either single repo or file
			if (input != "" && repositorySlug != "") || (input == "" && repositorySlug == "") {
				return fmt.Errorf("either --input or --repositorySlug must be specified, but not both")
			}

			client, err := bitbucket.NewClient(context.Background())
			if err != nil {
				return err
			}

			// Case 1: update from YAML file
			if input != "" {
				var parsed struct {
					Repositories []models.ExtendedRepository `yaml:"repositories"`
				}
				if err := utils.ParseFile(input, &parsed); err != nil {
					return err
				}
				updatedRepos, err := client.UpdateRepos(parsed.Repositories)
				if err != nil {
					client.Logger.Error(err.Error())
				}

				// Only print output if output format is specified
				if output != "" {
					if output != "yaml" && output != "json" {
						return fmt.Errorf("invalid output format: %s, allowed values: yaml, json", output)
					}
					return utils.PrintStructured("repositories", updatedRepos, output, "")
				}
				return nil
			}

			// Case 2: update single repository from CLI flags
			parts := strings.SplitN(repositorySlug, "/", 2)
			if len(parts) != 2 || parts[1] == "" {
				client.Logger.Error("Invalid repositorySlug format, expected <projectKey>/<repositorySlug>")
				return fmt.Errorf("invalid repositorySlug format, expected <projectKey>/<repositorySlug>")
			}
			projectKey := parts[0]
			slug := parts[1]

			repo := models.ExtendedRepository{
				ProjectKey:     projectKey,
				RepositorySlug: slug,
				RestRepository: &openapi.RestRepository{
					Slug:        &slug,
					Name:        utils.OptionalString(name),
					Description: utils.OptionalString(description),
					Project:     &openapi.RestChangesetRepositoryOriginProject{Key: newProjectKey},
				},
			}

			if defaultBranch != "" {
				repo.RestRepository.DefaultBranch = &defaultBranch
			}
			updatedRepos, err := client.UpdateRepos([]models.ExtendedRepository{repo})

			if err != nil {
				client.Logger.Error(err.Error())
			}

			// Only print output if output format is specified
			if output != "" {
				if output != "yaml" && output != "json" {
					return fmt.Errorf("invalid output format: %s, allowed values: yaml, json", output)
				}
				return utils.PrintStructured("repositories", updatedRepos, output, "")
			}
			return nil
		},
	}

	cmd.Flags().StringVarP(&repositorySlug, "repositorySlug", "s", "", "Identifier of the current repository in format <projectKey>/<repositorySlug> (required if --input not used)")
	cmd.Flags().StringVar(&name, "name", "", "Repository name (optional)")
	cmd.Flags().StringVar(&description, "desc", "", "Repository description (optional)")
	cmd.Flags().StringVar(&defaultBranch, "default-branch", "", "Default branch name (optional)")
	cmd.Flags().StringVarP(&input, "input", "i", "", `Path to YAML or JSON file with repositories to update, or '-' to read from stdin.
If specified, multiple repositories can be updated in a single operation.
The file must contain a list of repositories with at least the "projectKey" and "repositorySlug" fields.
Other fields are optional and will be updated only if provided.

Example YAML:
repositories:
  - projectKey: DEV
    repositorySlug: my-repo
    restRepository:
      name: "New Repo Name"
      description: "Updated description"
      defaultBranch: "develop"
`)
	cmd.Flags().StringVar(&newProjectKey, "newProjectKey", "", "New project key for moving the repository (optional)")
	cmd.Flags().StringVarP(&output, "output", "o", "", "Optional output format: yaml or json")

	return cmd
}
