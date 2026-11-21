package permission

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
	"github.com/vinisman/bbctl/internal/bitbucket"
	"github.com/vinisman/bbctl/internal/models"
	"github.com/vinisman/bbctl/utils"
)

// RevokePermissionCmd returns a command to revoke repository permissions.
func RevokePermissionCmd() *cobra.Command {
	var (
		repositorySlugs string
		users           string
		groups          string
		input           string
		output          string
	)

	cmd := &cobra.Command{
		Use:   "revoke",
		Short: "Revoke repository permissions",
		Long: `Revoke all repository permissions for users and groups.

The permission level is not required: revoking removes the subject's repository
permission entirely.`,
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
				repos, err = buildRepositories(repositorySlugs, users, groups, "")
				if err != nil {
					return err
				}
			}

			if err := validateRepositories(repos, false); err != nil {
				return err
			}

			client, err := bitbucket.NewClient(context.Background())
			if err != nil {
				return err
			}

			if err := client.RevokeRepositoryPermissions(repos); err != nil {
				client.Logger.Error(err.Error())
				return nil
			}

			return utils.PrintStructured("permissions", repositoryPermissionRows(repos), output, "projectKey,repositorySlug,type,name")
		},
	}

	cmd.Flags().StringVarP(&repositorySlugs, "repositorySlug", "s", "", "Repository identifiers in format <projectKey>/<repositorySlug>, multiple repositories can be comma-separated")
	cmd.Flags().StringVar(&users, "user", "", "Usernames to revoke permissions from, multiple users can be comma-separated")
	cmd.Flags().StringVar(&groups, "group", "", "Group names to revoke permissions from, multiple groups can be comma-separated")
	cmd.Flags().StringVarP(&input, "input", "i", "", `Input YAML or JSON file or '-' for stdin containing repositories
Example:
repositories:
  - projectKey: DEMO
    repositorySlug: app-web
    users:
      - name: jdoe
    groups:
      - name: developers
`)
	cmd.Flags().StringVarP(&output, "output", "o", "plain", "Output format: plain|yaml|json")

	return cmd
}
