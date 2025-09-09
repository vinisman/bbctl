package project

import (
	"context"
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"github.com/vinisman/bbctl/internal/bitbucket"
	"github.com/vinisman/bbctl/internal/models"
	"github.com/vinisman/bbctl/utils"
)

func NewDeleteCmd() *cobra.Command {
	var (
		key   string
		input string
	)

	cmd := &cobra.Command{
		Use:   "delete",
		Short: "Delete a Bitbucket project",
		Long:  `Delete one or more Bitbucket projects. Use with caution as this operation cannot be undone.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			// Validate input
			if (key != "" && input != "") || (key == "" && input == "") {
				return fmt.Errorf("either --key or --input must be specified, but not both")
			}

			client, err := bitbucket.NewClient(context.Background())
			if err != nil {
				return err
			}

			var keys []string

			// Case 1: keys from CLI
			if key != "" {
				keys = strings.Split(key, ",")
				for i := range keys {
					keys[i] = strings.TrimSpace(keys[i])
				}
			}

			// Case 2: keys from YAML or JSON
			if input != "" {
				var parsed models.ProjectYaml
				if err := utils.ParseFile(input, &parsed); err != nil {
					return err
				}
				if len(parsed.Projects) == 0 {
					return fmt.Errorf("no projects found in file %s", input)
				}

				// Extract project keys from the parsed projects
				for _, p := range parsed.Projects {
					if p.Key != nil {
						keys = append(keys, *p.Key)
					}
				}

				if len(keys) == 0 {
					return fmt.Errorf("no project keys found in file %s", input)
				}
			}

			// Run deletion
			if err := client.DeleteProjects(keys); err != nil {
				client.Logger.Error("Failed to delete projects", "error", err)
				//return err
			}

			return nil
		},
	}

	cmd.Flags().StringVarP(&key, "key", "k", "", "Comma-separated project keys (e.g. PRJ1,PRJ2)")
	cmd.Flags().StringVarP(&input, "input", "i", "", `Path to YAML or JSON file with projects to delete. Use '-' to read from stdin.
Example file content:
  projects:
    - key: PRJ1
    - key: PRJ2
    - key: PRJ3
`)
	return cmd
}
