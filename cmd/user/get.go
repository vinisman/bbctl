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

func GetCmd() *cobra.Command {
	var (
		name   string
		all    bool
		output string
		input  string
	)

	cmd := &cobra.Command{
		Use:   "get",
		Short: "Get users from Bitbucket",
		Long: `Get one or more users from Bitbucket. 
You must specify exactly one of the following options: 
  -n/--name (comma-separated usernames), 
  --all (fetch all users), 
  --input (YAML or JSON file with a list of users, or '-' to read from stdin).`,
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

			var users []openapi.RestApplicationUser

			switch {
			case all:
				users, err = client.GetAllUsers()
				if err != nil {
					client.Logger.Error(err.Error())
					return nil
				}
			case name != "":
				users, err = client.GetUsers(utils.ParseColumnsToLower(name))
				if err != nil {
					client.Logger.Error(err.Error())
					return nil
				}
			case input != "":
				var parsed models.UserYaml
				if err := utils.ParseFile(input, &parsed); err != nil {
					return fmt.Errorf("failed to parse file %s: %w", input, err)
				}
				if len(parsed.Users) == 0 {
					return fmt.Errorf("no users found in file %s", input)
				}

				// Extract usernames from the parsed users
				var usernames []string
				for _, u := range parsed.Users {
					if u.Name != "" {
						usernames = append(usernames, u.Name)
					}
				}

				if len(usernames) == 0 {
					return fmt.Errorf("no usernames found in file %s", input)
				}

				users, err = client.GetUsers(usernames)
				if err != nil {
					client.Logger.Error(err.Error())
					return nil
				}
			}

			if err := utils.PrintStructured("users", users, output, "name,displayName,emailAddress,active"); err != nil {
				return fmt.Errorf("failed to print output: %w", err)
			}
			return nil
		},
	}

	cmd.Flags().StringVarP(&name, "name", "n", "", "Comma-separated usernames")
	cmd.Flags().BoolVar(&all, "all", false, "Fetch all users")
	cmd.Flags().StringVarP(
		&output,
		"output",
		"o",
		"plain",
		`Output format: plain|yaml|json.
The "yaml" and "json" formats print the full available structure with all fields.`,
	)
	cmd.Flags().StringVarP(&input, "input", "i", "", `Path to YAML or JSON file with users to get, or "-" to read from stdin.
Example file content:
  users:
    - name: user1
    - name: user2
    - name: user3
`)
	return cmd
}
