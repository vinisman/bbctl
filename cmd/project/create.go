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

func NewCreateCmd() *cobra.Command {
	var (
		key         string
		name        string
		description string
		input       string
	)

	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create a project",
		Long: `Create one or more Bitbucket projects either from CLI flags or from a YAML file.

Note: If you encounter a 401 Unauthorized error, please use your username and password for authentication instead of a token. 
This is required for older Bitbucket versions that do not support token-based operations.`,
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

			// Case 1: create from CLI flags
			if key != "" {
				if name == "" {
					return fmt.Errorf("--name is required when creating project with --key")
				}
				p := openapi.RestProject{
					Key:         &key,
					Name:        &name,
					Description: &description,
				}
				projects = []openapi.RestProject{p}
			}

			// Case 2: create from YAML file
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

			if len(projects) > 0 {
				err = client.CreateProjects(projects)
				if err != nil {
					client.Logger.Error(err.Error())
					return nil
				}
			}
			return nil
		},
	}

	cmd.Flags().StringVarP(&key, "key", "k", "", "Project key (required if --input is not specified)")
	cmd.Flags().StringVar(&name, "name", "", "Project name")
	cmd.Flags().StringVar(&description, "description", "", "Project description")
	cmd.Flags().StringVarP(&input, "input", "i", "", `Path to YAML file with projects to create, or '-' to read from stdin.
Example file content:
projects:
  - key: Project1
    name: Demo Project
    description: Example demo project
  - key: Project2
`)

	return cmd
}
