package user

import (
	"github.com/spf13/cobra"
)

func UserCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "user",
		Short: "Manage Bitbucket users",
		Long:  "Manage users in Bitbucket Server/Data Center",
	}

	cmd.AddCommand(
		GetCmd(),
		CreateCmd(),
		UpdateCmd(),
		DeleteCmd(),
	)

	return cmd
}
