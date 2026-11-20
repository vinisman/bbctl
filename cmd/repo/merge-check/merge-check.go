package mergecheck

import (
	"github.com/spf13/cobra"
)

func RepoMergeCheckCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "merge-check",
		Short: "Manage ScriptRunner merge checks for repositories",
	}

	cmd.AddCommand(
		// get
		GetMergeCheckCmd(),

		// diff/apply
		DiffMergeCheckCmd(),
	)

	return cmd
}
