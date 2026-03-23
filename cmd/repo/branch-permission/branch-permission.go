package branchpermission

import (
	"github.com/spf13/cobra"
)

func RepoBranchPermissionCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "branch-permission",
		Short: "Manage branch permissions for repositories",
	}

	cmd.AddCommand(
		GetBranchPermissionCmd(),
		CreateBranchPermissionCmd(),
		DeleteBranchPermissionCmd(),
		UpdateBranchPermissionCmd(),
	)

	return cmd
}
