package reviewergroup

import (
	"context"
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"github.com/vinisman/bbctl/internal/bitbucket"
	"github.com/vinisman/bbctl/internal/models"
	"github.com/vinisman/bbctl/utils"
)

func GetReviewerGroupCmd() *cobra.Command {
	var (
		repositorySlug string
		output         string
		input          string
	)
	cmd := &cobra.Command{
		Use:   "get",
		Short: "Get list of reviewer groups for repository",
		Long:  "Get all reviewer groups configured for specified repositories",
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := bitbucket.NewClient(context.Background())
			if err != nil {
				return err
			}

			var repositories []models.ExtendedRepository

			if input != "" {
				var parsed models.RepositoryYaml
				if err := utils.ParseFile(input, &parsed); err != nil {
					return err
				}
				repositories = parsed.Repositories
			} else {
				if repositorySlug == "" {
					return fmt.Errorf("please specify --repositorySlug")
				}

				repositories = []models.ExtendedRepository{}
				items := strings.Split(repositorySlug, ",")
				for _, item := range items {
					item = strings.TrimSpace(item)
					parts := strings.SplitN(item, "/", 2)
					if len(parts) != 2 || parts[1] == "" {
						client.Logger.Error(fmt.Sprintf("invalid repository identifier format: %s", item))
						return fmt.Errorf("invalid repository identifier format: %s", item)
					}
					repositories = append(repositories, models.ExtendedRepository{
						ProjectKey:     parts[0],
						RepositorySlug: parts[1],
					})
				}
			}

			values, err := client.GetReviewerGroups(repositories)
			if err != nil {
				client.Logger.Error(err.Error())
				return nil
			}

			return utils.PrintStructured("repositories", values, output, "projectKey,repositorySlug,reviewerGroups.id,reviewerGroups.name")

		},
	}

	cmd.Flags().StringVarP(&repositorySlug, "repositorySlug", "s", "", "Repository identifiers in format <projectKey>/<repositorySlug>, multiple repositories can be comma-separated")
	cmd.Flags().StringVarP(&output, "output", "o", "plain", "Output format: plain|yaml|json")
	cmd.Flags().StringVarP(&input, "input", "i", "", `Input YAML or JSON file or '-' for stdin containing repositories
	Example:
repositories:
  - projectKey: project_1
    repositorySlug: repo1
	`)
	return cmd
}
