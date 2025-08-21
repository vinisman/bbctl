package requiredbuild

import (
	"github.com/spf13/cobra"
)

func RepoRequiredBuildCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "required-build",
		Short: "Manage required-builds for repositories",
	}

	cmd.AddCommand(
		GetRequiredBuildCmd(),
		CreateRequiredBuildCmd(),
		UpdateRequiredBuildCmd(),
		DeleteRequiredBuildCmd(),
	)

	return cmd
}
