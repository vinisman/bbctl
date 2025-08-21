package requiredbuild

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
	"github.com/vinisman/bbctl/internal/bitbucket"
	"github.com/vinisman/bbctl/utils"
)

func GetRequiredBuildCmd() *cobra.Command {
	var (
		projectKey string
		slug       string
		output     string
	)
	cmd := &cobra.Command{
		Use:   "get",
		Short: "Get list of required-builds for repositories",
		RunE: func(cmd *cobra.Command, args []string) error {
			if projectKey == "" || slug == "" {
				return fmt.Errorf("please specify --project and --slug")
			}

			client, err := bitbucket.NewClient(context.Background())
			if err != nil {
				return err
			}

			values, err := client.GetRequiredBuilds(projectKey, slug)
			if err != nil {
				return err
			}

			return utils.PrintStructured("requiredBuilds", values, output, "id")

		},
	}

	cmd.Flags().StringVarP(&projectKey, "projectKey", "k", "", "Project key (required)")
	cmd.Flags().StringVarP(&slug, "repositorySlug", "s", "", "Repository slug (required)")
	cmd.Flags().StringVarP(&output, "output", "o", "plain", "Output format: plain|yaml|json")

	return cmd
}
