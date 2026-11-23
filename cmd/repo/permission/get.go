package permission

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
	"github.com/vinisman/bbctl/internal/bitbucket"
	"github.com/vinisman/bbctl/internal/models"
	"github.com/vinisman/bbctl/utils"
)

// GetPermissionCmd returns a command to list repository permissions.
func GetPermissionCmd() *cobra.Command {
	var (
		repositorySlugs string
		input           string
		output          string
	)

	cmd := &cobra.Command{
		Use:   "get",
		Short: "Get repository permissions",
		Long:  "Get users and groups with permissions on the specified repositories.",
		RunE: func(cmd *cobra.Command, args []string) error {
			var repos []models.RepositoryPermissions

			if input != "" {
				var parsed models.RepositoryPermissionYaml
				if err := utils.ParseFile(input, &parsed); err != nil {
					return fmt.Errorf("failed to parse input file: %w", err)
				}
				repos = parsed.Repositories
			} else {
				if repositorySlugs == "" {
					return fmt.Errorf("please specify --repositorySlug or --input")
				}
				var err error
				repos, err = parseRepositories(repositorySlugs)
				if err != nil {
					return err
				}
			}

			if len(repos) == 0 {
				return fmt.Errorf("no repositories specified")
			}
			for _, r := range repos {
				if r.ProjectKey == "" || r.RepositorySlug == "" {
					return fmt.Errorf("projectKey and repositorySlug are required for each repository")
				}
			}

			client, err := bitbucket.NewClient(context.Background())
			if err != nil {
				return err
			}

			rows, err := client.GetRepositoryPermissions(repos)
			if err != nil {
				// Print whatever was fetched before the error so partial
				// results are not lost; the error goes to the log.
				client.Logger.Error(err.Error())
				if len(rows) == 0 {
					return nil
				}
			}

			return utils.PrintStructured("permissions", rows, output, "projectKey,repositorySlug,type,name,permission")
		},
	}

	cmd.Flags().StringVarP(&repositorySlugs, "repositorySlug", "s", "", "Repository identifiers in format <projectKey>/<repositorySlug>, multiple repositories can be comma-separated")
	cmd.Flags().StringVarP(&input, "input", "i", "", `Input YAML or JSON file or '-' for stdin containing repositories
Example:
repositories:
  - projectKey: DEMO
    repositorySlug: app-web
`)
	cmd.Flags().StringVarP(&output, "output", "o", "plain", "Output format: plain|yaml|json")

	return cmd
}
