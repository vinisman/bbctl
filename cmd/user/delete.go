package user

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
		Short: "Delete users",
		Long: `Delete one or more Bitbucket users either from CLI flags or from a YAML or JSON file.

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

			var usernames []string

			// Case 1: delete from CLI flag
			if name != "" {
				usernames = strings.Split(name, ",")
				// Trim whitespace from each username
				for i, username := range usernames {
					usernames[i] = strings.TrimSpace(username)
				}
			}

			// Case 2: delete from file
			if input != "" {
				var parsed models.UserYaml
				if err := utils.ParseFile(input, &parsed); err != nil {
					return fmt.Errorf("failed to parse file %s: %w", input, err)
				}
				if len(parsed.Users) == 0 {
					return fmt.Errorf("no users found in file %s", input)
				}

				// Extract usernames from the parsed users
				for _, u := range parsed.Users {
					if u.Name != "" {
						usernames = append(usernames, u.Name)
					}
				}

				if len(usernames) == 0 {
					return fmt.Errorf("no usernames found in file %s", input)
				}
			}

			// Delete users
			deletedUsers, err := client.DeleteUsers(usernames)
			if err != nil {
				client.Logger.Error(err.Error())
				return nil
			}

			// Print results
			if err := utils.PrintStructured("users", deletedUsers, output, "name,displayName,emailAddress"); err != nil {
				return fmt.Errorf("failed to print output: %w", err)
			}

			return nil
		},
	}

	cmd.Flags().StringVarP(&name, "name", "n", "", "Comma-separated usernames to delete")
	cmd.Flags().StringVarP(&input, "input", "i", "", `Path to YAML or JSON file with users to delete, or "-" to read from stdin.
Example file content:
  users:
    - name: user1
    - name: user2`)
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
