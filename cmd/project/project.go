package project

import (
	"github.com/spf13/cobra"
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
	)

	return cmd
}
