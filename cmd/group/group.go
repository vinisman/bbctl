package group

import (
	"github.com/spf13/cobra"
)

func GroupCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "group",
		Short: "Manage Bitbucket groups",
		Long:  "Manage groups in Bitbucket Server/Data Center",
	}

	cmd.AddCommand(
		GetCmd(),
		CreateCmd(),
		DeleteCmd(),
	)

	return cmd
}

