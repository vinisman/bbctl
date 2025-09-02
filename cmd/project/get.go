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

func NewGetCmd() *cobra.Command {
	var (
		key    string
		all    bool
		output string
		input  string
	)

	cmd := &cobra.Command{
		Use:   "get",
		Short: "Get project names from Bitbucket",
		Long: `Get one or more projects from Bitbucket. 
You must specify exactly one of the following options: 
  --key (comma-separated project keys), 
  --all (fetch all projects), 
  --input (YAML or JSON file with a list of projects, or '-' to read from stdin).`,
		RunE: func(cmd *cobra.Command, args []string) error {
			// Validate mutual exclusivity
			count := 0
			if key != "" {
				count++
			}
			if all {
				count++
			}
			if input != "" {
				count++
			}
			if count != 1 {
				return fmt.Errorf("please specify exactly one of --key, --all, or --input")
			}

			client, err := bitbucket.NewClient(context.Background())
			if err != nil {
				return err
			}

			var projects []openapi.RestProject

			switch {
			case all:
				projects, err = bitbucket.GetAllProjects(client)
				if err != nil {
					client.Logger.Error(err.Error())
					return nil
				}
			case key != "":
				projects, err = client.GetProjects(utils.ParseColumnsToLower(key))
				if err != nil {
					client.Logger.Error(err.Error())
					return nil
				}
			case input != "":
				var parsed models.ProjectList
				if err := utils.ParseFile(input, &parsed); err != nil {
					return fmt.Errorf("failed to parse file %s: %w", input, err)
				}
				if len(parsed.Projects) == 0 {
					return fmt.Errorf("no projects found in file %s", input)
				}

				projects, err = client.GetProjects(parsed.Projects)
				if err != nil {
					client.Logger.Error(err.Error())
					return nil
				}
			}

			if err := utils.PrintStructured("projects", projects, output, "id,name,key,description"); err != nil {
				return fmt.Errorf("failed to print output: %w", err)
			}
			return nil
		},
	}

	cmd.Flags().StringVarP(&key, "key", "k", "", "Comma-separated project keys")
	cmd.Flags().BoolVar(&all, "all", false, "Fetch all projects")
	cmd.Flags().StringVarP(
		&output,
		"output",
		"o",
		"plain",
		`Output format: plain|yaml|json.
The "yaml" and "json" formats print the full available structure with all fields.`,
	)
	cmd.Flags().StringVarP(&input, "input", "i", "", `Path to YAML or JSON file with projects to get, or "-" to read from stdin.
Example file content:
  projects:
    - projectKey1
    - projectKey2
    - projectKey3
`)
	return cmd
}
