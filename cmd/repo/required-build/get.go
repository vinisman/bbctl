package requiredbuild

import (
	"context"
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"github.com/vinisman/bbctl/internal/bitbucket"
	"github.com/vinisman/bbctl/utils"
)

func GetRequiredBuildCmd() *cobra.Command {
	var (
		repositorySlug string
		output         string
	)
	cmd := &cobra.Command{
		Use:   "get",
		Short: "Get list of required-builds for repositories",
		RunE: func(cmd *cobra.Command, args []string) error {
			if repositorySlug == "" {
				return fmt.Errorf("please specify --repositorySlug")
			}

			client, err := bitbucket.NewClient(context.Background())
			if err != nil {
				return err
			}

			parts := strings.SplitN(repositorySlug, "/", 2)
			if len(parts) != 2 || parts[1] == "" {
				client.Logger.Error("invalid repository identifier format, repository slug is empty")
				return fmt.Errorf("invalid repository identifier format, repository slug is empty")
			}
			projectKey := parts[0]
			slug := parts[1]

			values, err := client.GetRequiredBuilds(projectKey, slug)
			if err != nil {
				return err
			}

			return utils.PrintStructured("requiredBuilds", values, output, "id")

		},
	}

	cmd.Flags().StringVarP(&repositorySlug, "repositorySlug", "s", "", "Repository identifier in format <projectKey>/<repositorySlug>")
	cmd.Flags().StringVarP(&output, "output", "o", "plain", "Output format: plain|yaml|json")

	return cmd
}
