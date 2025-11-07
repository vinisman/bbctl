package repo

import (
	"github.com/spf13/cobra"
	requiredbuild "github.com/vinisman/bbctl/cmd/repo/required-build"
	reviewergroup "github.com/vinisman/bbctl/cmd/repo/reviewer-group"
	"github.com/vinisman/bbctl/cmd/repo/webhook"
	workzonecmd "github.com/vinisman/bbctl/cmd/repo/workzone"
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

		// reviewer-groups
		reviewergroup.RepoReviewerGroupCmd(),

		// workzone
		workzonecmd.RepoWorkzoneCmd(),
	)

	return cmd
}
