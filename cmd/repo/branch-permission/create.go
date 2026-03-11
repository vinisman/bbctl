package branchpermission

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
	"github.com/vinisman/bbctl/internal/bitbucket"
	"github.com/vinisman/bbctl/internal/models"
	"github.com/vinisman/bbctl/utils"
)

// CreateBranchPermissionCmd returns a cobra command to create branch permissions from a YAML file
func CreateBranchPermissionCmd() *cobra.Command {
	var input string
	var output string

	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create branch permissions from YAML file",
		Long: `Create one or multiple branch permissions from a YAML file.

Be careful: Bitbucket allows branch permissions with duplicate configurations,
so make sure to use unique matcher/type combinations to avoid confusion.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if input == "" {
				return fmt.Errorf("--input is required")
			}

			var parsed models.RepositoryYamlInput
			if err := utils.ParseFile(input, &parsed); err != nil {
				return fmt.Errorf("failed to parse input file: %w", err)
			}

			repos := parsed.ToRepositoryYaml()

			client, err := bitbucket.NewClient(context.Background())
			if err != nil {
				return err
			}

			if len(repos.Repositories) == 0 {
				client.Logger.Info("no repositories found in file", "file", input)
				return nil
			}

			hasPermissions := false
			for _, repo := range repos.Repositories {
				if repo.BranchPermissions != nil && len(*repo.BranchPermissions) > 0 {
					hasPermissions = true
					break
				}
			}

			if !hasPermissions {
				return fmt.Errorf("no branch permissions defined in file %s", input)
			}

			updatedRepos, err := client.CreateBranchPermissions(repos.Repositories)
			if err != nil {
				client.Logger.Error(err.Error())
			}

			// Only print output if output format is specified
			if output != "" {
				if output != "yaml" && output != "json" {
					return fmt.Errorf("invalid output format: %s, allowed values: yaml, json", output)
				}
				return utils.PrintStructured("repositories", updatedRepos, output, "")
			}

			return nil
		},
	}

	cmd.Flags().StringVarP(&input, "input", "i", "", `Path to YAML or JSON file or '-' to read from stdin
Example:
repositories:
  - projectKey: DEV
    repositorySlug: my-repo
    branchPermissions:
      - type: pull-request-only
        matcher:
          id: "refs/heads/main"
          displayId: "main"
          type: "BRANCH"
        users:
          - admin
        groups:
          - developers
`)
	cmd.Flags().StringVarP(&output, "output", "o", "", "Optional output format: yaml or json")

	return cmd
}
