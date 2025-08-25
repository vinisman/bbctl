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
		key  string
		file string
	)

	cmd := &cobra.Command{
		Use:   "delete",
		Short: "Delete a Bitbucket project",
		Long:  `Delete one or more Bitbucket projects. Use with caution as this operation cannot be undone.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			// Validate input
			if (key != "" && file != "") || (key == "" && file == "") {
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

			// Case 2: keys from YAML
			if file != "" {
				var parsed models.ProjectList
				if err := utils.ParseYAMLFile(file, &parsed); err != nil {
					return err
				}
				keys = parsed.Projects
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
	cmd.Flags().StringVarP(&file, "input", "i", "", `Path to YAML file with projects to delete.
Example file content:
  projects:
    - PRJ1
    - PRJ2
    - PRJ3
`)
	return cmd
}
