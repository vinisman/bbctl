package cmd

import (
	"log/slog"
	"os"

	"github.com/spf13/cobra"
	"github.com/vinisman/bbctl/cmd/apply"
	"github.com/vinisman/bbctl/cmd/create"
	"github.com/vinisman/bbctl/cmd/delete"
	"github.com/vinisman/bbctl/cmd/get"
	"github.com/vinisman/bbctl/utils"
)

var RootCmd = &cobra.Command{
	Use:   "bbctl",
	Short: "CLI tool for Bitbucket repositories management",
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		var err error
		utils.Cfg, err = utils.LoadConfig()
		if err != nil {
			return err
		}

		level := slog.LevelInfo
		if utils.Debug {
			level = slog.LevelDebug
		}
		utils.Logger = slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: level}))
		return nil
	},
}

func Execute() {
	if err := RootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func init() {
	RootCmd.PersistentFlags().BoolVar(&utils.Debug, "debug", false, "enable debug logging")
	RootCmd.PersistentFlags().StringVar(&utils.GlobalURL, "base-url", "", "Bitbucket base URL (or env BITBUCKET_BASE_URL)")
	RootCmd.PersistentFlags().StringVar(&utils.GlobalToken, "token", "", "Bitbucket token (or env BITBUCKET_TOKEN)")
	RootCmd.PersistentFlags().IntVar(&utils.PageSize, "page-size", 50, "Page size for repository listing")

	RootCmd.CompletionOptions.DisableDefaultCmd = true

	// GET
	RootCmd.AddCommand(get.GetCmd)

	// CREATE
	RootCmd.AddCommand(create.CreateCmd)

	// APPLY
	RootCmd.AddCommand(apply.ApplyCmd)

	// DELETE
	RootCmd.AddCommand(delete.DeleteCmd)
}
