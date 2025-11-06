package reviewergroup

import (
	"github.com/spf13/cobra"
)

func RepoReviewerGroupCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "reviewer-group",
		Short: "Manage reviewer groups for repositories",
		Long:  "Manage reviewer groups in Bitbucket repositories. Reviewer groups can be used to set default reviewers for pull requests.",
	}

	cmd.AddCommand(
		GetReviewerGroupCmd(),
		CreateReviewerGroupCmd(),
		UpdateReviewerGroupCmd(),
		DeleteReviewerGroupCmd(),
	)

	return cmd
}

