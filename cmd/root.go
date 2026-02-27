package cmd

import (
	"fmt"
	"log/slog"
	"os"

	"github.com/joho/godotenv"
	"github.com/spf13/cobra"
	"github.com/vinisman/bbctl/cmd/group"
	"github.com/vinisman/bbctl/cmd/project"
	"github.com/vinisman/bbctl/cmd/repo"
	"github.com/vinisman/bbctl/cmd/user"
	"github.com/vinisman/bbctl/cmd/validate"
	"github.com/vinisman/bbctl/internal/config"
)

var (
	debug bool

	// Global flags for authentication and URL
	flagURL        string
	flagToken      string
	flagUsername   string
	flagPassword   string
	flagPageSize   int
	flagMaxWorkers int

	// Version and Commit are set at build time via -ldflags
	Version string
	Commit  string
)

func NewRootCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "bbctl",
		Short: "Bitbucket Data Center CLI",
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			if cmd.Name() == "version" {
				return nil
			}

			// Load environment variables from .env file
			_ = godotenv.Load()

			// Load config from environment
			c, err := config.LoadConfig()
			if err != nil {
				return err
			}

			// Override config with flags if provided
			if flagURL != "" {
				c.BaseURL = flagURL
			}
			if flagToken != "" {
				c.Token = flagToken
			}
			if flagUsername != "" {
				c.Username = flagUsername
			}
			if flagPassword != "" {
				c.Password = flagPassword
			}

			if flagPageSize > 0 {
				c.PageSize = flagPageSize
			}

			if flagMaxWorkers > 0 {
				config.GlobalMaxWorkers = flagMaxWorkers
			}

			// Validate authentication
			if c.Token == "" && (c.Username == "" || c.Password == "") {
				return fmt.Errorf("either token or username/password must be provided")
			}

			config.GlobalCfg = c

			// Initialize logger
			level := slog.LevelInfo
			if debug {
				level = slog.LevelDebug
			}
			config.GlobalLogger = slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: level}))
			return nil
		},
	}

	// Global flags with clear descriptions
	cmd.PersistentFlags().BoolVar(&debug, "debug", false, "Enable debug mode (verbose logging)")
	cmd.PersistentFlags().StringVar(&flagURL, "url", "", "Bitbucket Base URL (overrides BITBUCKET_BASE_URL)")
	cmd.PersistentFlags().StringVar(&flagToken, "token", "", "Bitbucket API token (overrides BITBUCKET_TOKEN)")
	cmd.PersistentFlags().StringVarP(&flagUsername, "username", "u", "", "Bitbucket username (overrides BITBUCKET_USERNAME)")
	cmd.PersistentFlags().StringVarP(&flagPassword, "password", "p", "", "Bitbucket password (overrides BITBUCKET_PASSWORD)")
	cmd.PersistentFlags().IntVar(&flagPageSize, "page-size", 0, "Page size for API requests (overrides BITBUCKET_PAGE_SIZE, default 50)")
	cmd.PersistentFlags().IntVar(&flagMaxWorkers, "max-workers", 0, "Maximum number of concurrent workers (overrides BITBUCKET_MAX_WORKERS, default 5)")

	// Add subcommands
	cmd.AddCommand(
		repo.NewRepoCmd(),
		project.NewProjectCmd(),
		user.UserCmd(),
		group.GroupCmd(),
		validate.NewValidateCmd(),
		versionCmd(),
	)

	return cmd
}

func versionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print the version number of bbctl",
		Run: func(cmd *cobra.Command, args []string) {
			if Version != "" && Commit != "" {
				fmt.Printf("bbctl version: %s, commit: %s\n", Version, Commit)
			} else {
				fmt.Println("bbctl version: unknown")
			}
		},
	}
}
