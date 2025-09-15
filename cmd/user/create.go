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

func CreateCmd() *cobra.Command {
	var (
		name        string
		displayName string
		email       string
		password    string
		input       string
		output      string
	)

	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create a user",
		Long: `Create one or more Bitbucket users either from CLI flags or from a YAML or JSON file.

Note: If you encounter a 401 Unauthorized error, please use your username and password for authentication instead of a token. 
This is required for older Bitbucket versions that do not support token-based operations.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			// Validate input
			if (name != "" && input != "") || (name == "" && input == "") {
				return fmt.Errorf("either --name and --displayName or --input must be specified, but not both")
			}

			client, err := bitbucket.NewClient(context.Background())
			if err != nil {
				return err
			}

			var users []openapi.RestApplicationUser

			// Case 1: create from CLI flags
			if name != "" {
				if displayName == "" {
					return fmt.Errorf("--displayName is required when creating user with --name")
				}
				if email == "" {
					return fmt.Errorf("--email is required when creating user with --name")
				}
				if password == "" {
					return fmt.Errorf("--user-password is required when creating user with --name")
				}
				u := openapi.RestApplicationUser{
					Name:         &name,
					DisplayName:  &displayName,
					EmailAddress: &email,
				}
				users = append(users, u)
			}

			// Case 2: create from file
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
					if user.DisplayName == "" {
						return fmt.Errorf("user at index %d is missing required field 'displayName'", i)
					}
					if user.EmailAddress == "" {
						return fmt.Errorf("user at index %d is missing required field 'emailAddress'", i)
					}
				}

				// Convert from models.User to openapi.RestApplicationUser
				users = make([]openapi.RestApplicationUser, len(parsed.Users))
				for i, user := range parsed.Users {
					users[i] = openapi.RestApplicationUser{
						Name:         &user.Name,
						DisplayName:  &user.DisplayName,
						EmailAddress: &user.EmailAddress,
					}
				}
			}

			// Create users
			var passwords []string
			if name != "" {
				// CLI mode - use provided password
				passwords = []string{password}
			} else {
				// File mode - password must be provided via --user-password
				if password == "" {
					return fmt.Errorf("--user-password is required when creating users from file")
				}
				// Use the same password for all users from file
				passwords = make([]string, len(users))
				for i := range passwords {
					passwords[i] = password
				}
			}

			createdUsers, err := client.CreateUsers(users, passwords)
			if err != nil {
				client.Logger.Error(err.Error())
				return nil
			}

			// Print results
			if err := utils.PrintStructured("users", createdUsers, output, "name,displayName,emailAddress,active"); err != nil {
				return fmt.Errorf("failed to print output: %w", err)
			}

			return nil
		},
	}

	cmd.Flags().StringVarP(&name, "name", "n", "", "Username (required)")
	cmd.Flags().StringVarP(&displayName, "displayName", "d", "", "Display name (required)")
	cmd.Flags().StringVarP(&email, "email", "e", "", "Email address (required)")
	cmd.Flags().StringVar(&password, "user-password", "", "User password (required)")
	cmd.Flags().StringVarP(&input, "input", "i", "", `Path to YAML or JSON file with users to create, or "-" to read from stdin.
Example file content:
  users:
    - name: user1
      displayName: "User One"
      emailAddress: user1@example.com
    - name: user2
      displayName: "User Two"
      emailAddress: user2@example.com`)
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
