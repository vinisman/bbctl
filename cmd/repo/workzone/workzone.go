package workzone

import (
	"github.com/spf13/cobra"
)

// RepoWorkzoneCmd returns the top-level "workzone" command for repositories
func RepoWorkzoneCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "workzone",
		Short: "Manage Workzone settings for repositories",
	}

	cmd.AddCommand(
		GetWorkzoneCmd(),
		SetWorkzoneCmd(),
		UpdateWorkzoneCmd(),
		DeleteWorkzoneCmd(),
	)

	return cmd
}
