package project

import (
	"github.com/spf13/cobra"
	"github.com/vinisman/bbctl/cmd/project/permission"
)

func NewProjectCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "project",
		Short: "Manage Bitbucket projects",
	}

	cmd.AddCommand(
		NewGetCmd(),
		NewCreateCmd(),
		NewUpdateCmd(),
		NewDeleteCmd(),

		// permissions
		permission.ProjectPermissionCmd(),
	)

	return cmd
}
