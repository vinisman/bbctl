package reviewergroup

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

// DeleteReviewerGroupCmd returns a cobra command to delete reviewer groups from a YAML file or flags
func DeleteReviewerGroupCmd() *cobra.Command {
	var input string
	var projectKey string
	var repositorySlug string
	var ids string

	cmd := &cobra.Command{
		Use:   "delete",
		Short: "Delete reviewer groups from YAML file by Id or from flags",
		RunE: func(cmd *cobra.Command, args []string) error {
			if input == "" && (projectKey == "" || repositorySlug == "" || ids == "") {
				return fmt.Errorf("either --input or all of --projectKey, --repositorySlug, and --ids must be provided")
			}
			if input != "" && (projectKey != "" || repositorySlug != "" || ids != "") {
				return fmt.Errorf("--input cannot be used together with --projectKey, --repositorySlug, or --ids")
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
				var reviewerGroups []openapi.RestReviewerGroup
				for _, idStr := range idStrs {
					idStr = strings.TrimSpace(idStr)
					if idStr == "" {
						continue
					}
					idInt, err := strconv.ParseInt(idStr, 10, 64)
					if err != nil {
						return fmt.Errorf("invalid reviewer group id '%s': %w", idStr, err)
					}
					reviewerGroups = append(reviewerGroups, openapi.RestReviewerGroup{Id: &idInt})
				}
				if len(reviewerGroups) == 0 {
					return fmt.Errorf("no valid reviewer group ids provided")
				}

				repositories = []models.ExtendedRepository{
					{
						ProjectKey:     projectKey,
						RepositorySlug: repositorySlug,
						ReviewerGroups: &reviewerGroups,
					},
				}
			}

			client, err := bitbucket.NewClient(context.Background())
			if err != nil {
				return err
			}

			_, err = client.DeleteReviewerGroups(repositories)
			if err != nil {
				client.Logger.Error(err.Error())
			}

			return nil
		},
	}

	cmd.Flags().StringVarP(&input, "input", "i", "", `Path to YAML or JSON file with repositories and reviewer groups to delete (use '-' to read from stdin)
Example:
  repositories:
    - projectKey: DEV
      repositorySlug: my-repo
      reviewerGroups:
        - id: 123
`)
	cmd.Flags().StringVarP(&projectKey, "projectKey", "k", "", "Project key of the repository")
	cmd.Flags().StringVarP(&repositorySlug, "repositorySlug", "s", "", "Repository slug")
	cmd.Flags().StringVar(&ids, "ids", "", "Comma-separated list of reviewer group IDs to delete")

	return cmd
}
