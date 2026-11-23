package permission

import (
	"github.com/spf13/cobra"
)

// RepoPermissionCmd returns the parent command for repository permissions.
func RepoPermissionCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "permission",
		Short: "Manage repository permissions",
		Long:  "Get, grant and revoke repository permissions for users and groups.",
	}

	cmd.AddCommand(
		GetPermissionCmd(),
		GrantPermissionCmd(),
		RevokePermissionCmd(),
	)

	return cmd
}
