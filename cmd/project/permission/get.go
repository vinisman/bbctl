package permission

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
	"github.com/vinisman/bbctl/internal/bitbucket"
	"github.com/vinisman/bbctl/internal/models"
	"github.com/vinisman/bbctl/utils"
)

// GetPermissionCmd returns a command to list project permissions.
func GetPermissionCmd() *cobra.Command {
	var (
		projectKeys string
		input       string
		output      string
	)

	cmd := &cobra.Command{
		Use:   "get",
		Short: "Get project permissions",
		Long:  "Get users and groups with permissions on the specified projects.",
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
				projects = parseProjectKeys(projectKeys)
			}

			if len(projects) == 0 {
				return fmt.Errorf("no projects specified")
			}
			for _, p := range projects {
				if p.ProjectKey == "" {
					return fmt.Errorf("projectKey is required for each project")
				}
			}

			client, err := bitbucket.NewClient(context.Background())
			if err != nil {
				return err
			}

			rows, err := client.GetProjectPermissions(projects)
			if err != nil {
				client.Logger.Error(err.Error())
				return nil
			}

			return utils.PrintStructured("permissions", rows, output, "projectKey,type,name,permission")
		},
	}

	cmd.Flags().StringVarP(&projectKeys, "projectKey", "k", "", "Project keys, multiple projects can be comma-separated")
	cmd.Flags().StringVarP(&input, "input", "i", "", `Input YAML or JSON file or '-' for stdin containing projects
Example:
projects:
  - projectKey: DEMO
`)
	cmd.Flags().StringVarP(&output, "output", "o", "plain", "Output format: plain|yaml|json")

	return cmd
}
