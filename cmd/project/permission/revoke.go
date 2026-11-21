package permission

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
	"github.com/vinisman/bbctl/internal/bitbucket"
	"github.com/vinisman/bbctl/internal/models"
	"github.com/vinisman/bbctl/utils"
)

// RevokePermissionCmd returns a command to revoke project permissions.
func RevokePermissionCmd() *cobra.Command {
	var (
		projectKeys string
		users       string
		groups      string
		input       string
		output      string
	)

	cmd := &cobra.Command{
		Use:   "revoke",
		Short: "Revoke project permissions",
		Long: `Revoke all project permissions for users and groups.

The permission level is not required: revoking removes the subject's project
permission entirely.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			var projects []models.ProjectPermissions

			if input != "" {
				var parsed models.ProjectPermissionYaml
				if err := utils.ParseFile(input, &parsed); err != nil {
					return fmt.Errorf("failed to parse input file: %w", err)
				}
				projects = parsed.Projects
			} else {
				if projectKeys == "" {
					return fmt.Errorf("please specify --projectKey or --input")
				}
				if users == "" && groups == "" {
					return fmt.Errorf("please specify --user and/or --group")
				}
				projects = buildProjects(projectKeys, users, groups, "")
			}

			if err := validateProjects(projects, false); err != nil {
				return err
			}

			client, err := bitbucket.NewClient(context.Background())
			if err != nil {
				return err
			}

			if err := client.RevokeProjectPermissions(projects); err != nil {
				client.Logger.Error(err.Error())
				return nil
			}

			return utils.PrintStructured("permissions", projectPermissionRows(projects), output, "projectKey,type,name")
		},
	}

	cmd.Flags().StringVarP(&projectKeys, "projectKey", "k", "", "Project keys, multiple projects can be comma-separated")
	cmd.Flags().StringVar(&users, "user", "", "Usernames to revoke permissions from, multiple users can be comma-separated")
	cmd.Flags().StringVar(&groups, "group", "", "Group names to revoke permissions from, multiple groups can be comma-separated")
	cmd.Flags().StringVarP(&input, "input", "i", "", `Input YAML or JSON file or '-' for stdin containing projects
Example:
projects:
  - projectKey: DEMO
    users:
      - name: jdoe
    groups:
      - name: developers
`)
	cmd.Flags().StringVarP(&output, "output", "o", "plain", "Output format: plain|yaml|json")

	return cmd
}
