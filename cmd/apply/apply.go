package apply

import (
	"github.com/spf13/cobra"
)

var ApplyCmd = &cobra.Command{
	Use:   "apply",
	Short: "Create or update a resource from a file or from stdin",
	Long: `Create or update a resource from a file or from stdin.

Available subcommands:
  repo   Create a repository	
  repos   Bulk create repositories
`,
}

func init() {
	ApplyCmd.AddCommand(applyReposCmd)
	ApplyCmd.AddCommand(applyRepoCmd)
	ApplyCmd.AddCommand(applyRepoWebhookCmd)
	ApplyCmd.AddCommand(applyRepoWebhooksCmd)
}
