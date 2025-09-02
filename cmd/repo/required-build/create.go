package requiredbuild

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
	"github.com/vinisman/bbctl/internal/bitbucket"
	"github.com/vinisman/bbctl/internal/models"
	"github.com/vinisman/bbctl/utils"
)

func CreateRequiredBuildCmd() *cobra.Command {
	var input string
	var output string

	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create required-builds from YAML file",
		Long: `Create one or multiple required-builds from a YAML file.

Be careful: Bitbucket allows required-builds with duplicate names, 
so make sure to use unique names to avoid confusion or accidental overwrites.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if input == "" {
				return fmt.Errorf("--input is required")
			}

			var parsed models.RepositoryYaml
			if err := utils.ParseFile(input, &parsed); err != nil {
				return fmt.Errorf("failed to parse file: %w", err)
			}

			if len(parsed.Repositories) == 0 {
				return fmt.Errorf("no required-builds found in file %s", input)
			}

			hasRequiredBuilds := false
			for _, repo := range parsed.Repositories {
				if len(repo.RequiredBuilds) > 0 {
					hasRequiredBuilds = true
					break
				}
			}
			if !hasRequiredBuilds {
				return fmt.Errorf("no required-builds defined in file %s", input)
			}

			client, err := bitbucket.NewClient(context.Background())
			if err != nil {
				return err
			}

			updatedRepos, err := client.CreateRequiredBuilds(parsed.Repositories)
			if err != nil {
				client.Logger.Error(err.Error())
			}

			if output != "yaml" && output != "json" {
				return fmt.Errorf("invalid output format: %s, allowed values: yaml, json", output)
			}

			return utils.PrintStructured("repositories", updatedRepos, output, "")

		},
	}

	cmd.Flags().StringVarP(&input, "input", "i", "", `Path to YAML or JSON file with webhooks to create
Use "-" to read the file from stdin
Example:
repositories:
  - projectKey: project_1
    repositorySlug: repo1
    requiredBuilds:  
      - buildparentkeys:
          - job1
          - job2
        exemptrefmatcher: null
        refmatcher:
          displayid: ANY_REF_MATCHER_ID
          id: ANY_REF_MATCHER_ID
          type:
              id: ANY_REF
              name: Any branch
`)

	cmd.Flags().StringVarP(&output, "output", "o", "yaml", "Output format: yaml or json (default: yaml)")

	return cmd
}
