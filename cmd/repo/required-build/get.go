package requiredbuild

import (
	"context"
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"github.com/vinisman/bbctl/internal/bitbucket"
	"github.com/vinisman/bbctl/internal/models"
	"github.com/vinisman/bbctl/utils"
)

func GetRequiredBuildCmd() *cobra.Command {
	var (
		repositorySlug string
		output         string
		input          string
	)
	cmd := &cobra.Command{
		Use:   "get",
		Short: "Get list of required-builds for repositories",
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := bitbucket.NewClient(context.Background())
			if err != nil {
				return err
			}

			var repositories []models.ExtendedRepository

			if input != "" {
				var parsed models.RepositoryYaml
				if err := utils.ParseYAMLFile(input, &parsed); err != nil {
					return err
				}
				repositories = parsed.Repositories
			} else {
				if repositorySlug == "" {
					return fmt.Errorf("please specify --repositorySlug")
				}

				parts := strings.SplitN(repositorySlug, "/", 2)
				if len(parts) != 2 || parts[1] == "" {
					client.Logger.Error("invalid repository identifier format, repository slug is empty")
					return fmt.Errorf("invalid repository identifier format, repository slug is empty")
				}
				repositories = []models.ExtendedRepository{
					{
						ProjectKey:     parts[0],
						RepositorySlug: parts[1],
					},
				}
			}

			values, err := client.GetRequiredBuilds(repositories)
			if err != nil {
				client.Logger.Error(err.Error())
				return nil
			}
			return utils.PrintStructured("requiredBuilds", values, output, "id")

		},
	}

	cmd.Flags().StringVarP(&repositorySlug, "repositorySlug", "s", "", "Repository identifier in format <projectKey>/<repositorySlug>")
	cmd.Flags().StringVarP(&output, "output", "o", "plain", "Output format: plain|yaml|json")
	cmd.Flags().StringVarP(&input, "input", "i", "", `Input YAML file or '-' for stdin containing repositories
	Example:
repositories:
  - projectKey: project_1
    repositorySlug: repo1
	`)

	return cmd
}
