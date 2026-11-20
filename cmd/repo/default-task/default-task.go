package defaulttask

import (
	"github.com/spf13/cobra"
)

func RepoDefaultTaskCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "default-task",
		Short: "Manage repository default tasks",
	}

	cmd.AddCommand(
		// get
		GetDefaultTaskCmd(),

		// diff/apply
		DiffDefaultTaskCmd(),
	)

	return cmd
}
