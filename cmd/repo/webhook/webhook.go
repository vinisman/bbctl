package webhook

import (
	"github.com/spf13/cobra"
)

func RepoWebHookCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "webhook",
		Short: "Manage webhooks for repositories",
	}

	cmd.AddCommand(
		GetWebHookCmd(),
		CreateWebHookCmd(),
		DeleteWebHookCmd(),
		UpdateWebHookCmd(),
		DiffWebHookCmd(),
	)

	return cmd
}
