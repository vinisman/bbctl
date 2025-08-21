package webhook

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
	"github.com/vinisman/bbctl/internal/bitbucket"
	"github.com/vinisman/bbctl/utils"
)

func GetWebHookCmd() *cobra.Command {
	var (
		projectKey string
		slug       string
		output     string
	)
	cmd := &cobra.Command{
		Use:   "get",
		Short: "Get list of webhooks for repository",
		RunE: func(cmd *cobra.Command, args []string) error {
			if projectKey == "" || slug == "" {
				return fmt.Errorf("please specify --projectKey and --repositorySlug")
			}

			client, err := bitbucket.NewClient(context.Background())
			if err != nil {
				return err
			}

			values, err := client.GetWebhooks(projectKey, slug)
			if err != nil {
				return err
			}

			return utils.PrintStructured("webhooks", values, output, "id,name")

		},
	}

	cmd.Flags().StringVarP(&projectKey, "projectKey", "k", "", "Project key (required)")
	cmd.Flags().StringVarP(&slug, "repositorySlug", "s", "", "Repository slug (required)")
	cmd.Flags().StringVarP(&output, "output", "o", "plain", "Output format: plain|yaml|json")

	return cmd
}
