package group

import (
	"context"
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"github.com/vinisman/bbctl/internal/bitbucket"
	"github.com/vinisman/bbctl/internal/models"
	"github.com/vinisman/bbctl/utils"
)

func DeleteCmd() *cobra.Command {
	var (
		name   string
		input  string
		output string
	)

	cmd := &cobra.Command{
		Use:   "delete",
		Short: "Delete groups",
		Long: `Delete one or more Bitbucket groups either from CLI flags or from a YAML or JSON file.

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

			var groupNames []string

			// Case 1: delete from CLI flag
			if name != "" {
				groupNames = strings.Split(name, ",")
				// Trim whitespace from each group name
				for i, groupName := range groupNames {
					groupNames[i] = strings.TrimSpace(groupName)
				}
			}

			// Case 2: delete from file
			if input != "" {
				var parsed models.GroupYaml
				if err := utils.ParseFile(input, &parsed); err != nil {
					return fmt.Errorf("failed to parse file %s: %w", input, err)
				}
				if len(parsed.Groups) == 0 {
					return fmt.Errorf("no groups found in file %s", input)
				}

				// Extract group names from the parsed groups
				for _, g := range parsed.Groups {
					if g.Name != "" {
						groupNames = append(groupNames, g.Name)
					}
				}

				if len(groupNames) == 0 {
					return fmt.Errorf("no group names found in file %s", input)
				}
			}

			// Delete groups
			deletedGroups, err := client.DeleteGroups(groupNames)
			if err != nil {
				client.Logger.Error(err.Error())
				return nil
			}

			// Print results
			if err := utils.PrintStructured("groups", deletedGroups, output, "name"); err != nil {
				return fmt.Errorf("failed to print output: %w", err)
			}

			return nil
		},
	}

	cmd.Flags().StringVarP(&name, "name", "n", "", "Comma-separated group names to delete")
	cmd.Flags().StringVarP(&input, "input", "i", "", `Path to YAML or JSON file with groups to delete, or "-" to read from stdin.
Example file content:
  groups:
    - name: developers
    - name: testers`)
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
