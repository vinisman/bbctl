package workzone

import (
	"context"
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	bb "github.com/vinisman/bbctl/internal/bitbucket"
	wz "github.com/vinisman/bbctl/internal/workzone"
	"github.com/vinisman/bbctl/utils"
)

var (
	setSections  []string
	setRepoIdent string
	setInputPath string
)

// SetWorkzoneCmd returns a cobra command to set workzone settings
func SetWorkzoneCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "set",
		Short: "Set Workzone settings for repositories",
		Long: `Set Workzone settings for one or more repositories.

Sections (plugin tab names):
  - properties: repository workflow properties
  - reviewers: branch reviewers list
  - signatures: branch sign approvers list
  - mergerules: branch automergers list

Notes:
  - This command requires --input with payload data for selected sections.
  - You can specify multiple sections via comma-separated or repeated --section flags.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if setInputPath == "" {
				return fmt.Errorf("please specify --input with payload")
			}

			client, err := bb.NewClient(context.Background())
			if err != nil {
				return err
			}
			wzClient := wz.NewClient(client)

			repos, err := utils.ParseRepositoriesFromArgs(setRepoIdent, setInputPath)
			if err != nil {
				return err
			}

			normalized := map[string]bool{}
			if len(setSections) == 0 {
				return fmt.Errorf("please specify --section (properties, reviewers, signatures, mergerules)")
			}
			for _, s := range setSections {
				s = strings.TrimSpace(strings.ToLower(s))
				switch s {
				case "properties", "reviewers", "signatures", "mergerules":
					normalized[s] = true
				default:
					return fmt.Errorf("unsupported --section: %s (supported: properties, reviewers, signatures, mergerules)", s)
				}
			}

			successCount := 0
			totalSections := 0

			if normalized["properties"] {
				totalSections++
				if err := wzClient.SetReposWorkflowProperties(repos); err != nil {
					client.Logger.Error(err.Error())
				} else {
					successCount++
					client.Logger.Info(fmt.Sprintf("Successfully set workflow properties for %d repositories", len(repos)))
				}
			}
			if normalized["reviewers"] {
				totalSections++
				if err := wzClient.SetReposReviewersList(repos); err != nil {
					client.Logger.Error(err.Error())
				} else {
					successCount++
					client.Logger.Info(fmt.Sprintf("Successfully set reviewers for %d repositories", len(repos)))
				}
			}
			if normalized["signatures"] {
				totalSections++
				if err := wzClient.SetReposSignapprovers(repos); err != nil {
					client.Logger.Error(err.Error())
				} else {
					successCount++
					client.Logger.Info(fmt.Sprintf("Successfully set sign approvers for %d repositories", len(repos)))
				}
			}
			if normalized["mergerules"] {
				totalSections++
				if err := wzClient.SetReposAutomergers(repos); err != nil {
					client.Logger.Error(err.Error())
				} else {
					successCount++
					client.Logger.Info(fmt.Sprintf("Successfully set mergerules for %d repositories", len(repos)))
				}
			}

			if successCount == totalSections {
				client.Logger.Info(fmt.Sprintf("All %d sections completed successfully for %d repositories", totalSections, len(repos)))
			} else if successCount > 0 {
				client.Logger.Warn(fmt.Sprintf("Completed %d/%d sections successfully for %d repositories", successCount, totalSections, len(repos)))
			}

			return nil
		},
	}

	cmd.Flags().StringSliceVar(&setSections, "section", []string{}, "Sections to set (repeatable or comma-separated): properties|reviewers|signatures|mergerules")
	cmd.Flags().StringVarP(&setRepoIdent, "repositorySlug", "s", "", "Repository identifier <projectKey>/<repoSlug> (optional when using --input)")
	cmd.Flags().StringVarP(&setInputPath, "input", "i", "", `Input YAML or JSON file or '-' for stdin with repositories and payload
Example:
repositories:
  - projectKey: project_1
    repositorySlug: repo1
`)
	return cmd
}

// parseSetRepos is deprecated; use utils.ParseRepositoriesFromArgs instead
