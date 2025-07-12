package cmd

import (
	"github.com/spf13/cobra"
	"github.com/vinisman/bbctl/cmd/apply"
	"github.com/vinisman/bbctl/cmd/create"
	"github.com/vinisman/bbctl/cmd/delete"
	"github.com/vinisman/bbctl/cmd/get"
	"github.com/vinisman/bbctl/utils"
)

var RootCmd = &cobra.Command{
	Use:               "bbctl",
	Short:             "bbctl — CLI tool for bulk management of repositories on Bitbucket Server/Data Center",
	CompletionOptions: cobra.CompletionOptions{DisableDefaultCmd: true},
}

var debug bool

func init() {
	// Global flags
	RootCmd.PersistentFlags().StringVar(&utils.GlobalURL, "url", "", "Bitbucket base URL (can use BITBUCKET_URL)")
	RootCmd.PersistentFlags().StringVar(&utils.GlobalToken, "token", "", "Bitbucket token (can use BITBUCKET_TOKEN)")
	RootCmd.PersistentFlags().StringVar(&utils.EnvFile, "env", "", "Path to .env file")
	RootCmd.PersistentFlags().StringVarP(&utils.ProjectKey, "project", "p", "", "Bitbucket project key (global)")
	RootCmd.PersistentFlags().BoolVarP(&debug, "debug", "d", false, "Enable debug logging")
	RootCmd.PersistentPreRun = func(cmd *cobra.Command, args []string) {
		utils.InitLogger(debug)
	}
	RootCmd.MarkPersistentFlagRequired("project")

	// Create
	createCmd := &cobra.Command{Use: "create", Short: "Create resources"}
	createCmd.AddCommand(create.ReposCmd)
	createCmd.AddCommand(create.BuildsCmd)
	RootCmd.AddCommand(createCmd)

	// Apply
	applyCmd := &cobra.Command{Use: "apply", Short: "Create or update resources"}
	applyCmd.AddCommand(apply.ReposCmd)
	RootCmd.AddCommand(applyCmd)

	// Delete
	deleteCmd := &cobra.Command{Use: "delete", Short: "Delete resources"}
	deleteCmd.AddCommand(delete.ReposCmd)
	deleteCmd.AddCommand(delete.BuildsCmd)
	RootCmd.AddCommand(deleteCmd)

	// Get
	getCmd := &cobra.Command{Use: "get", Short: "Get resources"}
	getCmd.AddCommand(get.ReposCmd)
	getCmd.AddCommand(get.RepoCmd)
	RootCmd.AddCommand(getCmd)
}
