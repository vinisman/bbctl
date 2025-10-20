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
	updateSections  []string
	updateRepoIdent string
	updateInputPath string
)

// UpdateWorkzoneCmd updates Workzone settings for repositories
func UpdateWorkzoneCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "update",
		Short: "Update Workzone settings for repositories",
		Long: `Update Workzone settings for one or more repositories.

Sections (plugin tab names):
  - properties: update repository workflow properties
  - reviewers: replace branch reviewers list (set semantics)
  - signatures: replace branch sign approvers list (set semantics)
  - mergerules: replace branch automergers list (set semantics)

Notes:
  - Requires --input with payload for selected sections.
  - Multiple sections allowed via comma-separated or repeated --section.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if updateInputPath == "" {
				return fmt.Errorf("please specify --input with payload")
			}

			client, err := bb.NewClient(context.Background())
			if err != nil {
				return err
			}
			wzClient := wz.NewClient(client)

			repos, err := utils.ParseRepositoriesFromArgs(updateRepoIdent, updateInputPath)
			if err != nil {
				return err
			}

			normalized := map[string]bool{}
			if len(updateSections) == 0 {
				return fmt.Errorf("please specify --section (properties, reviewers, signatures, mergerules)")
			}
			for _, s := range updateSections {
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
				if err := wzClient.UpdateReposWorkflowProperties(repos); err != nil {
					client.Logger.Error(err.Error())
				} else {
					successCount++
					client.Logger.Info(fmt.Sprintf("Successfully updated workflow properties for %d repositories", len(repos)))
				}
			}
			if normalized["reviewers"] {
				totalSections++
				if err := wzClient.SetReposReviewersList(repos); err != nil {
					client.Logger.Error(err.Error())
				} else {
					successCount++
					client.Logger.Info(fmt.Sprintf("Successfully updated reviewers for %d repositories", len(repos)))
				}
			}
			if normalized["signatures"] {
				totalSections++
				if err := wzClient.SetReposSignapprovers(repos); err != nil {
					client.Logger.Error(err.Error())
				} else {
					successCount++
					client.Logger.Info(fmt.Sprintf("Successfully updated sign approvers for %d repositories", len(repos)))
				}
			}
			if normalized["mergerules"] {
				totalSections++
				if err := wzClient.SetReposAutomergers(repos); err != nil {
					client.Logger.Error(err.Error())
				} else {
					successCount++
					client.Logger.Info(fmt.Sprintf("Successfully updated mergerules for %d repositories", len(repos)))
				}
			}

			if successCount == totalSections {
				client.Logger.Info(fmt.Sprintf("All %d sections updated successfully for %d repositories", totalSections, len(repos)))
			} else if successCount > 0 {
				client.Logger.Warn(fmt.Sprintf("Updated %d/%d sections successfully for %d repositories", successCount, totalSections, len(repos)))
			}

			return nil
		},
	}

	cmd.Flags().StringSliceVar(&updateSections, "section", []string{}, "Sections to update (repeatable or comma-separated): properties|reviewers|signatures|mergerules")
	cmd.Flags().StringVarP(&updateRepoIdent, "repositorySlug", "s", "", "Repository identifier <projectKey>/<repoSlug> (optional when using --input)")
	cmd.Flags().StringVarP(&updateInputPath, "input", "i", "", `Input YAML or JSON file or '-' for stdin with repositories and payload
Example:
repositories:
  - projectKey: project_1
    repositorySlug: repo1
`)
	return cmd
}
