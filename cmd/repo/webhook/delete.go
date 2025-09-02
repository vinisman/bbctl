package webhook

import (
	"context"
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
	"github.com/vinisman/bbctl/internal/bitbucket"
	"github.com/vinisman/bbctl/internal/models"
	"github.com/vinisman/bbctl/utils"
	"github.com/vinisman/bitbucket-sdk-go/openapi"
)

// DeleteWebHookCmd returns a cobra command to delete webhooks from a YAML file or flags
func DeleteWebHookCmd() *cobra.Command {
	var input string
	var projectKey string
	var repositorySlug string
	var ids string

	cmd := &cobra.Command{
		Use:   "delete",
		Short: "Delete webhooks from YAML file by Id or from flags",
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
					return fmt.Errorf("no repositories found in file %s", input)
				}

				repositories = parsed.Repositories

			} else {
				idStrs := strings.Split(ids, ",")
				var webhooks []openapi.RestWebhook
				for _, idStr := range idStrs {
					idStr = strings.TrimSpace(idStr)
					if idStr == "" {
						continue
					}
					idInt, err := strconv.ParseInt(idStr, 10, 64)
					if err != nil {
						return fmt.Errorf("invalid webhook id '%s': %w", idStr, err)
					}
					if idInt < math.MinInt32 || idInt > math.MaxInt32 {
						return fmt.Errorf("ID %d out of int32 range", idInt)
					}
					id32 := int32(idInt)
					webhooks = append(webhooks, openapi.RestWebhook{Id: &id32})
				}
				if len(webhooks) == 0 {
					return fmt.Errorf("no valid webhook ids provided")
				}

				repositories = []models.ExtendedRepository{
					{
						ProjectKey:     projectKey,
						RepositorySlug: repositorySlug,
						Webhooks:       webhooks,
					},
				}
			}

			client, err := bitbucket.NewClient(context.Background())
			if err != nil {
				return err
			}

			err = client.DeleteWebhooks(repositories)
			if err != nil {
				client.Logger.Error(err.Error())
			}

			return nil
		},
	}

	cmd.Flags().StringVarP(&input, "input", "i", "", `Path to YAML or JSON file with repositories and webhooks to delete (use '-' to read from stdin)
Example:
  repositories:
    - projectKey: DEV
      repositorySlug: my-repo
      webhooks:
        - id: 123
`)
	cmd.Flags().StringVarP(&projectKey, "projectKey", "k", "", "Project key of the repository")
	cmd.Flags().StringVarP(&repositorySlug, "repositorySlug", "s", "", "Repository slug")
	cmd.Flags().StringVar(&ids, "ids", "", "Comma-separated list of webhook IDs to delete")

	return cmd
}
