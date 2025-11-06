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

func GetCmd() *cobra.Command {
	var (
		name   string
		all    bool
		output string
		input  string
	)

	cmd := &cobra.Command{
		Use:   "get",
		Short: "Get groups from Bitbucket",
		Long: `Get one or more groups from Bitbucket. 
You must specify exactly one of the following options: 
  -n/--name (comma-separated group names), 
  --all (fetch all groups), 
  --input (YAML or JSON file with a list of groups, or '-' to read from stdin).`,
		RunE: func(cmd *cobra.Command, args []string) error {
			// Validate mutual exclusivity
			count := 0
			if name != "" {
				count++
			}
			if all {
				count++
			}
			if input != "" {
				count++
			}
			if count != 1 {
				return fmt.Errorf("please specify exactly one of -n/--name, --all, or --input")
			}

			client, err := bitbucket.NewClient(context.Background())
			if err != nil {
				return err
			}

			var groups []openapi.RestDetailedGroup

			switch {
			case all:
				groups, err = client.GetAllGroups()
				if err != nil {
					client.Logger.Error(err.Error())
					return nil
				}
			case name != "":
				groups, err = client.GetGroups(utils.ParseColumnsToLower(name))
				if err != nil {
					client.Logger.Error(err.Error())
					return nil
				}
			case input != "":
				var parsed models.GroupYaml
				if err := utils.ParseFile(input, &parsed); err != nil {
					return fmt.Errorf("failed to parse file %s: %w", input, err)
				}
				if len(parsed.Groups) == 0 {
					return fmt.Errorf("no groups found in file %s", input)
				}

				// Extract group names from the parsed groups
				var groupNames []string
				for _, g := range parsed.Groups {
					if g.Name != "" {
						groupNames = append(groupNames, g.Name)
					}
				}

				if len(groupNames) == 0 {
					return fmt.Errorf("no group names found in file %s", input)
				}

				groups, err = client.GetGroups(groupNames)
				if err != nil {
					client.Logger.Error(err.Error())
					return nil
				}
			}

			if err := utils.PrintStructured("groups", groups, output, "name"); err != nil {
				return fmt.Errorf("failed to print output: %w", err)
			}
			return nil
		},
	}

	cmd.Flags().StringVarP(&name, "name", "n", "", "Comma-separated group names")
	cmd.Flags().BoolVar(&all, "all", false, "Fetch all groups")
	cmd.Flags().StringVarP(
		&output,
		"output",
		"o",
		"plain",
		`Output format: plain|yaml|json.
The "yaml" and "json" formats print the full available structure with all fields.`,
	)
	cmd.Flags().StringVarP(&input, "input", "i", "", `Path to YAML or JSON file with groups to get, or "-" to read from stdin.
Example file content:
  groups:
    - name: developers
    - name: administrators
    - name: testers
`)
	return cmd
}

