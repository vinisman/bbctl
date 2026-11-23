package permission

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
	"github.com/vinisman/bbctl/internal/bitbucket"
	"github.com/vinisman/bbctl/internal/models"
	"github.com/vinisman/bbctl/utils"
)

// GrantPermissionCmd returns a command to grant repository permissions.
func GrantPermissionCmd() *cobra.Command {
	var (
		repositorySlugs string
		users           string
		groups          string
		permission      string
		input           string
		output          string
	)

	cmd := &cobra.Command{
		Use:   "grant",
		Short: "Grant repository permissions",
		Long: `Grant or update repository permissions for users and groups.

Permission levels: REPO_READ, REPO_WRITE, REPO_ADMIN.`,
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
				if users == "" && groups == "" {
					return fmt.Errorf("please specify --user and/or --group")
				}
				var err error
				repos, err = buildRepositories(repositorySlugs, users, groups, permission)
				if err != nil {
					return err
				}
			}

			if err := validateRepositories(repos, true); err != nil {
				return err
			}

			client, err := bitbucket.NewClient(context.Background())
			if err != nil {
				return err
			}

			if err := client.GrantRepositoryPermissions(repos); err != nil {
				client.Logger.Error(err.Error())
				return nil
			}

			return utils.PrintStructured("permissions", repositoryPermissionRows(repos), output, "projectKey,repositorySlug,type,name,permission")
		},
	}

	cmd.Flags().StringVarP(&repositorySlugs, "repositorySlug", "s", "", "Repository identifiers in format <projectKey>/<repositorySlug>, multiple repositories can be comma-separated")
	cmd.Flags().StringVar(&users, "user", "", "Usernames to grant permissions to, multiple users can be comma-separated")
	cmd.Flags().StringVar(&groups, "group", "", "Group names to grant permissions to, multiple groups can be comma-separated")
	cmd.Flags().StringVar(&permission, "permission", "", "Permission level: REPO_READ|REPO_WRITE|REPO_ADMIN")
	cmd.Flags().StringVarP(&input, "input", "i", "", `Input YAML or JSON file or '-' for stdin containing repositories
Example:
repositories:
  - projectKey: DEMO
    repositorySlug: app-web
    users:
      - name: jdoe
        permission: REPO_READ
    groups:
      - name: developers
        permission: REPO_WRITE
`)
	cmd.Flags().StringVarP(&output, "output", "o", "plain", "Output format: plain|yaml|json")

	return cmd
}
