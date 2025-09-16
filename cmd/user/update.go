package user

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
	"github.com/vinisman/bbctl/internal/bitbucket"
	"github.com/vinisman/bbctl/internal/models"
	"github.com/vinisman/bbctl/utils"
	openapi "github.com/vinisman/bitbucket-sdk-go/openapi"
)

func UpdateCmd() *cobra.Command {
	var (
		name        string
		displayName string
		email       string
		input       string
		output      string
	)

	cmd := &cobra.Command{
		Use:   "update",
		Short: "Update users",
		Long: `Update one or more Bitbucket users either from CLI flags or from a YAML or JSON file.

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

			var users []openapi.RestApplicationUser

			// Case 1: update from CLI flags
			if name != "" {
				if displayName == "" && email == "" {
					return fmt.Errorf("at least one of --displayName or --email must be specified when updating user with --name")
				}

				u := openapi.RestApplicationUser{
					Name: &name,
				}
				if displayName != "" {
					u.DisplayName = &displayName
				}
				if email != "" {
					u.EmailAddress = &email
				}
				users = append(users, u)
			}

			// Case 2: update from file
			if input != "" {
				var parsed models.UserYaml
				if err := utils.ParseFile(input, &parsed); err != nil {
					return fmt.Errorf("failed to parse file %s: %w", input, err)
				}
				if len(parsed.Users) == 0 {
					return fmt.Errorf("no users found in file %s", input)
				}

				// Validate that all users have required fields
				for i, user := range parsed.Users {
					if user.Name == "" {
						return fmt.Errorf("user at index %d is missing required field 'name'", i)
					}
					if user.DisplayName == "" && user.EmailAddress == "" {
						return fmt.Errorf("user at index %d must have at least one of 'displayName' or 'emailAddress'", i)
					}
				}

				// Convert from models.User to openapi.RestApplicationUser
				users = make([]openapi.RestApplicationUser, len(parsed.Users))
				for i, user := range parsed.Users {
					users[i] = openapi.RestApplicationUser{
						Name: &user.Name,
					}
					if user.DisplayName != "" {
						users[i].DisplayName = &user.DisplayName
					}
					if user.EmailAddress != "" {
						users[i].EmailAddress = &user.EmailAddress
					}
				}
			}

			// Update users
			updatedUsers, err := client.UpdateUsers(users)
			if err != nil {
				client.Logger.Error(err.Error())
				return nil
			}

			// Print results
			if err := utils.PrintStructured("users", updatedUsers, output, "name,displayName,emailAddress"); err != nil {
				return fmt.Errorf("failed to print output: %w", err)
			}

			return nil
		},
	}

	cmd.Flags().StringVarP(&name, "name", "n", "", "Username to update (required)")
	cmd.Flags().StringVarP(&displayName, "displayName", "d", "", "New display name")
	cmd.Flags().StringVarP(&email, "email", "e", "", "New email address")
	cmd.Flags().StringVarP(&input, "input", "i", "", `Path to YAML or JSON file with users to update, or "-" to read from stdin.
Example file content:
  users:
    - name: user1
      displayName: "Updated User One"
      emailAddress: user1@newdomain.com
    - name: user2
      displayName: "Updated User Two"`)
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
