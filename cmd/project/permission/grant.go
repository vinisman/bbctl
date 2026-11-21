package permission

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
	"github.com/vinisman/bbctl/internal/bitbucket"
	"github.com/vinisman/bbctl/internal/models"
	"github.com/vinisman/bbctl/utils"
)

// GrantPermissionCmd returns a command to grant project permissions.
func GrantPermissionCmd() *cobra.Command {
	var (
		projectKeys string
		users       string
		groups      string
		permission  string
		input       string
		output      string
	)

	cmd := &cobra.Command{
		Use:   "grant",
		Short: "Grant project permissions",
		Long: `Grant or update project permissions for users and groups.

Permission levels: PROJECT_READ, PROJECT_WRITE, PROJECT_ADMIN.`,
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
				projects = buildProjects(projectKeys, users, groups, permission)
			}

			if err := validateProjects(projects, true); err != nil {
				return err
			}

			client, err := bitbucket.NewClient(context.Background())
			if err != nil {
				return err
			}

			if err := client.GrantProjectPermissions(projects); err != nil {
				client.Logger.Error(err.Error())
				return nil
			}

			return utils.PrintStructured("permissions", projectPermissionRows(projects), output, "projectKey,type,name,permission")
		},
	}

	cmd.Flags().StringVarP(&projectKeys, "projectKey", "k", "", "Project keys, multiple projects can be comma-separated")
	cmd.Flags().StringVar(&users, "user", "", "Usernames to grant permissions to, multiple users can be comma-separated")
	cmd.Flags().StringVar(&groups, "group", "", "Group names to grant permissions to, multiple groups can be comma-separated")
	cmd.Flags().StringVar(&permission, "permission", "", "Permission level: PROJECT_READ|PROJECT_WRITE|PROJECT_ADMIN")
	cmd.Flags().StringVarP(&input, "input", "i", "", `Input YAML or JSON file or '-' for stdin containing projects
Example:
projects:
  - projectKey: DEMO
    users:
      - name: jdoe
        permission: PROJECT_READ
    groups:
      - name: developers
        permission: PROJECT_WRITE
`)
	cmd.Flags().StringVarP(&output, "output", "o", "plain", "Output format: plain|yaml|json")

	return cmd
}
