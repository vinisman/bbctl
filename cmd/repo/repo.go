package repo

import (
	"github.com/spf13/cobra"
	branchpermission "github.com/vinisman/bbctl/cmd/repo/branch-permission"
	defaulttask "github.com/vinisman/bbctl/cmd/repo/default-task"
	mergecheck "github.com/vinisman/bbctl/cmd/repo/merge-check"
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

		// branch-permissions
		branchpermission.RepoBranchPermissionCmd(),

		// required-builds
		requiredbuild.RepoRequiredBuildCmd(),

		// ScriptRunner merge checks
		mergecheck.RepoMergeCheckCmd(),

		// default tasks
		defaulttask.RepoDefaultTaskCmd(),

		// reviewer-groups
		reviewergroup.RepoReviewerGroupCmd(),

		// workzone
		workzonecmd.RepoWorkzoneCmd(),
	)

	return cmd
}
