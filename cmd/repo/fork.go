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

func NewForkCmd() *cobra.Command {
	var (
		repositorySlug   string
		newProjectKey    string
		newName          string
		newDefaultBranch string
		newDescription   string
		input            string
	)

	cmd := &cobra.Command{
		Use:   "fork",
		Short: "Fork a repository",
		Long: `Fork a single repository or multiple repositories defined in a YAML file.
You must specify either --repositorySlug with --newProjectKey for a single repository,
or --input for a YAML file containing multiple forks.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			// Validate input: either single fork or file
			if (input != "" && repositorySlug != "") ||
				(input == "" && repositorySlug == "") {
				return fmt.Errorf("either --input, or --repositorySlug must be specified, but not both")
			}

			client, err := bitbucket.NewClient(context.Background())
			if err != nil {
				return err
			}

			// Case 1: fork from YAML file
			if input != "" {
				var parsed struct {
					Repositories []models.ExtendedRepository `yaml:"repositories"`
				}
				if err := utils.ParseFile(input, &parsed); err != nil {
					return fmt.Errorf("failed to parse YAML file: %w", err)
				}
				return client.ForkRepos(parsed.Repositories)
			}

			// Case 2: fork single repository
			parts := strings.SplitN(repositorySlug, "/", 2)
			if len(parts) != 2 || parts[1] == "" {
				client.Logger.Error("invalid repository identifier format, repository slug is empty")
				return fmt.Errorf("invalid repository identifier format, repository slug is empty")
			}
			sourceProjectKey := parts[0]
			sourceSlug := parts[1]

			// Build RestRepository with only specified fields (leave others nil)
			restRepo := openapi.RestRepository{}

			if newName != "" {
				restRepo.Name = &newName
			}
			if newDescription != "" {
				restRepo.Description = &newDescription
			}
			if newDefaultBranch != "" {
				restRepo.DefaultBranch = &newDefaultBranch
			}
			if newProjectKey != "" {
				restRepo.Project = &openapi.RestChangesetRepositoryOriginProject{Key: newProjectKey}
			} else {
				restRepo.Project = &openapi.RestChangesetRepositoryOriginProject{Key: sourceProjectKey}
			}
			// If at least one key is set, set ScmId and Forkable to defaults if you want, or leave nil.
			// (Instruction: only set given keys, so leave others nil.)
			repo := models.ExtendedRepository{
				ProjectKey:     sourceProjectKey,
				RepositorySlug: sourceSlug,
				RestRepository: restRepo,
			}

			err = client.ForkRepos([]models.ExtendedRepository{repo})
			if err != nil {
				client.Logger.Error(err.Error())
			}

			fmt.Printf("Forked repository: %s -> %s\n", repositorySlug, newProjectKey)
			return nil
		},
	}

	cmd.Flags().StringVarP(&repositorySlug, "repositorySlug", "s", "", "Repository identifier in format <projectKey>/<repositorySlug> of the repository to fork (required if --input not used)")
	cmd.Flags().StringVar(&newProjectKey, "newProjectKey", "", "New project key where the fork will be created (required if --input not used)")
	cmd.Flags().StringVar(&newName, "newName", "", "New name of the forked repository (optional, defaults to current repository name if not set)")
	cmd.Flags().StringVar(&newDefaultBranch, "newDefaultBranch", "", "New default branch for the forked repository (optional)")
	cmd.Flags().StringVar(&newDescription, "newDescription", "", "New description for the forked repository (optional)")
	cmd.Flags().StringVarP(&input, "input", "i", "", `Path to YAML or JSON file with forks to create, or '-' to read from stdin.
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
