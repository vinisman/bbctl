package group

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
	"github.com/vinisman/bbctl/internal/bitbucket"
	"github.com/vinisman/bbctl/internal/models"
	"github.com/vinisman/bbctl/utils"
	openapi "github.com/vinisman/bitbucket-sdk-go/openapi"
)

func CreateCmd() *cobra.Command {
	var (
		name   string
		input  string
		output string
	)

	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create a group",
		Long: `Create one or more Bitbucket groups either from CLI flags or from a YAML or JSON file.

Note: If you encounter a 401 Unauthorized error, please use your username and password for authentication instead of a token. 
This is required for older Bitbucket versions that do not support token-based operations.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			// Validate input
			if (name != "" && input != "") || (name == "" && input == "") {
				return fmt.Errorf("either --name or --input must be specified, but not both")
			}

			client, err := bitbucket.NewClient(context.Background())
			if err != nil {
				return err
			}

			var groups []openapi.RestDetailedGroup

			// Case 1: create from CLI flags
			if name != "" {
				g := openapi.RestDetailedGroup{
					Name: &name,
				}
				groups = append(groups, g)
			}

			// Case 2: create from file
			if input != "" {
				var parsed models.GroupYaml
				if err := utils.ParseFile(input, &parsed); err != nil {
					return fmt.Errorf("failed to parse file %s: %w", input, err)
				}
				if len(parsed.Groups) == 0 {
					return fmt.Errorf("no groups found in file %s", input)
				}

				// Validate that all groups have required fields
				for i, group := range parsed.Groups {
					if group.Name == "" {
						return fmt.Errorf("group at index %d is missing required field 'name'", i)
					}

					g := openapi.RestDetailedGroup{
						Name: &group.Name,
					}
					groups = append(groups, g)
				}
			}

			createdGroups, err := client.CreateGroups(groups)
			if err != nil {
				client.Logger.Error(err.Error())
				return nil
			}

			client.Logger.Info("Successfully created groups", "count", len(createdGroups))

			if err := utils.PrintStructured("groups", createdGroups, output, "name"); err != nil {
				return fmt.Errorf("failed to print output: %w", err)
			}
			return nil
		},
	}

	cmd.Flags().StringVarP(&name, "name", "n", "", "Group name")
	cmd.Flags().StringVarP(&input, "input", "i", "", `Path to YAML or JSON file with groups to create, or "-" to read from stdin.
Example file content:
  groups:
    - name: developers
    - name: testers
    - name: administrators
`)
	cmd.Flags().StringVarP(
		&output,
		"output",
		"o",
		"plain",
		`Output format: plain|yaml|json.
The "yaml" and "json" formats print the full available structure with all fields.`,
	)
	return cmd
}

