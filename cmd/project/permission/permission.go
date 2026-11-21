package permission

import (
	"github.com/spf13/cobra"
)

// ProjectPermissionCmd returns the parent command for project permissions.
func ProjectPermissionCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "permission",
		Short: "Manage project permissions",
		Long:  "Get, grant and revoke project permissions for users and groups.",
	}

	cmd.AddCommand(
		GetPermissionCmd(),
		GrantPermissionCmd(),
		RevokePermissionCmd(),
	)

	return cmd
}
