package requiredbuild

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
	"github.com/vinisman/bbctl/internal/bitbucket"
	"github.com/vinisman/bbctl/internal/models"
	"github.com/vinisman/bbctl/utils"
	"github.com/vinisman/bitbucket-sdk-go/openapi"
)

// DeleteWebHookCmd returns a cobra command to delete webhooks from a YAML file
func DeleteRequiredBuildCmd() *cobra.Command {
	var input string
	var project string
	var repo string
	var ids string

	cmd := &cobra.Command{
		Use:   "delete",
		Short: "Delete required-builds from YAML file by Id or by project, repo and ids",
		RunE: func(cmd *cobra.Command, args []string) error {
			if input == "" && (project == "" || repo == "" || ids == "") {
				return fmt.Errorf("either --input or --project, --repo and --ids must be provided")
			}

			if input != "" && (project != "" || repo != "" || ids != "") {
				return fmt.Errorf("cannot use --input together with --project, --repo or --ids")
			}

			var repositories []models.ExtendedRepository

			if input != "" {
				var parsed models.RepositoryYaml
				if err := utils.ParseFile(input, &parsed); err != nil {
					return fmt.Errorf("failed to parse file: %w", err)
				}

				if len(parsed.Repositories) == 0 {
					client, _ := bitbucket.NewClient(context.Background())
					if client != nil {
						client.Logger.Info("no repositories found in file", "file", input)
					}
					return nil
				}

				repositories = parsed.Repositories
			} else {
				idStrs := strings.Split(ids, ",")
				var requiredBuilds []openapi.RestRequiredBuildCondition
				for _, idStr := range idStrs {
					idStr = strings.TrimSpace(idStr)
					if idStr == "" {
						continue
					}
					idInt, err := strconv.ParseInt(idStr, 10, 64)
					if err != nil {
						return fmt.Errorf("invalid id '%s' in --ids: %w", idStr, err)
					}
					requiredBuilds = append(requiredBuilds, openapi.RestRequiredBuildCondition{Id: &idInt})
				}

				if len(requiredBuilds) == 0 {
					return fmt.Errorf("no valid ids provided in --ids")
				}

				repositories = []models.ExtendedRepository{
					{
						ProjectKey:     project,
						RepositorySlug: repo,
						RequiredBuilds: requiredBuilds,
					},
				}
			}

			client, err := bitbucket.NewClient(context.Background())
			if err != nil {
				return err
			}

			err = client.DeleteRequiredBuilds(repositories)
			if err != nil {
				client.Logger.Error(err.Error())
			}
			return nil
		},
	}

	cmd.Flags().StringVarP(&input, "input", "i", "", `Path to YAML or JSON file with repositories and webhooks to delete.
Passing '-' will read the input from stdin.
Example:
repositories:
  - projectKey: project_1
    repositorySlug: repo1
    requiredBuilds:  
      - id: 1
`)

	cmd.Flags().StringVarP(&project, "projectKey", "k", "", "Project key of the repository")
	cmd.Flags().StringVarP(&repo, "repositorySlug", "s", "", "Repository slug")
	cmd.Flags().StringVar(&ids, "ids", "", "Comma-separated list of required build IDs to delete")

	return cmd
}
