package repo

import (
	"github.com/spf13/cobra"
	requiredbuild "github.com/vinisman/bbctl/cmd/repo/required-build"
	"github.com/vinisman/bbctl/cmd/repo/webhook"
)

func NewRepoCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "repo",
		Short: "Manage Bitbucket repositories",
	}

	cmd.AddCommand(

		// main commands
		NewGetCmd(),
		NewCreateCmd(),
		NewUpdateCmd(),
		NewDeleteCmd(),
		NewForkCmd(),

		// webhooks
		webhook.RepoWebHookCmd(),

		// required-builds
		requiredbuild.RepoRequiredBuildCmd(),
	)

	return cmd
}
