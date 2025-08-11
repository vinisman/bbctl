package create

import (
	"github.com/spf13/cobra"
)

var CreateCmd = &cobra.Command{
	Use:   "create",
	Short: "Create a resource from a file or from stdin",
	Long: `Create a resource from a file or from stdin.

Available subcommands:
  repo   Create a repository	
  repos   Bulk create repositories
`,
}

func init() {
	CreateCmd.AddCommand(createRepoCmd)
	CreateCmd.AddCommand(createReposCmd)
}
