package delete

import (
	"github.com/spf13/cobra"
)

var DeleteCmd = &cobra.Command{
	Use:   "delete",
	Short: "Delete a resource from a file or from stdin",
	Long: `Delete a resource from a file or from stdin.

Available subcommands:
  repo   Delete a repository	
  repos   Bulk delete repositories
`,
}

func init() {
	DeleteCmd.AddCommand(deleteRepoCmd)
	DeleteCmd.AddCommand(deleteReposCmd)
}
