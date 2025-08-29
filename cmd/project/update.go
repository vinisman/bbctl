package project

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
	"github.com/vinisman/bbctl/internal/bitbucket"
	"github.com/vinisman/bbctl/internal/models"
	"github.com/vinisman/bbctl/utils"
	openapi "github.com/vinisman/bitbucket-sdk-go/openapi"
)

func NewUpdateCmd() *cobra.Command {
	var (
		key         string
		name        string
		description string
		input       string
	)

	cmd := &cobra.Command{
		Use:   "update",
		Short: "Update a project",
		Long:  `Update one or more Bitbucket projects either from CLI flags or from a YAML file.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			// Validate input
			if (key != "" && input != "") || (key == "" && input == "") {
				return fmt.Errorf("either --key and --name or --input must be specified, but not both")
			}

			client, err := bitbucket.NewClient(context.Background())
			if err != nil {
				return err
			}

			var projects []openapi.RestProject

			// Case 1: update from CLI flags
			if key != "" {
				if name == "" {
					return fmt.Errorf("--name is required when updating project with --key")
				}
				p := openapi.RestProject{
					Key:         &key,
					Name:        &name,
					Description: &description,
				}
				projects = []openapi.RestProject{p}
			}

			// Case 2: update from YAML file
			if input != "" {
				var parsed models.ProjectYaml
				if err := utils.ParseYAMLFile(input, &parsed); err != nil {
					return fmt.Errorf("failed to parse YAML file %s: %w", input, err)
				}

				if len(parsed.Projects) == 0 {
					return fmt.Errorf("no projects found in file %s", input)
				}

				projects = parsed.Projects
			}

			// Only call UpdateProjects once, if any projects were collected
			if len(projects) > 0 {
				err = client.UpdateProjects(projects)
				if err != nil {
					client.Logger.Error(err.Error())
					return nil
				}
			}

			return nil
		},
	}

	cmd.Flags().StringVarP(&key, "key", "k", "", "Project key (required if --input is not specified)")
	cmd.Flags().StringVar(&name, "name", "", "Project name (required)")
	cmd.Flags().StringVar(&description, "des", "", "Project description")
	cmd.Flags().StringVarP(&input, "input", "i", "", `Path to YAML file with projects to update, or '-' to read from stdin.
Example file content:
projects:
  - key: DEMO
    name: Demo Project
    description: Example demo project
  - key: TEST
    name: Test Project
    description: Updated description
`)

	return cmd
}
