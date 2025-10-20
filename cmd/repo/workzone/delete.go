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
	deleteSections  []string
	deleteRepoIdent string
	deleteInputPath string
)

// DeleteWorkzoneCmd deletes Workzone settings for repositories
func DeleteWorkzoneCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "delete",
		Short: "Delete Workzone settings for repositories",
		Long: `Delete Workzone settings for one or more repositories.

Sections (plugin tab names):
  - properties: remove repository workflow properties
  - reviewers: delete branch reviewers list
  - signatures: delete branch sign approvers list
  - mergerules: delete branch automergers list

Notes:
  - For delete operations, payload is not required; repo list can be provided via --repositorySlug or --input.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := bb.NewClient(context.Background())
			if err != nil {
				return err
			}
			wzClient := wz.NewClient(client)

			repos, err := utils.ParseRepositoriesFromArgs(deleteRepoIdent, deleteInputPath)
			if err != nil {
				return err
			}

			normalized := map[string]bool{}
			if len(deleteSections) == 0 {
				return fmt.Errorf("please specify --section (properties, reviewers, signatures, mergerules)")
			}
			for _, s := range deleteSections {
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
				if err := wzClient.RemoveReposWorkflowProperties(repos); err != nil {
					client.Logger.Error(err.Error())
				} else {
					successCount++
					client.Logger.Info(fmt.Sprintf("Successfully removed workflow properties for %d repositories", len(repos)))
				}
			}
			if normalized["reviewers"] {
				totalSections++
				if err := wzClient.DeleteReposReviewersList(repos); err != nil {
					client.Logger.Error(err.Error())
				} else {
					successCount++
					client.Logger.Info(fmt.Sprintf("Successfully deleted reviewers for %d repositories", len(repos)))
				}
			}
			if normalized["signatures"] {
				totalSections++
				if err := wzClient.DeleteReposSignapprovers(repos); err != nil {
					client.Logger.Error(err.Error())
				} else {
					successCount++
					client.Logger.Info(fmt.Sprintf("Successfully deleted sign approvers for %d repositories", len(repos)))
				}
			}
			if normalized["mergerules"] {
				totalSections++
				if err := wzClient.DeleteReposAutomergers(repos); err != nil {
					client.Logger.Error(err.Error())
				} else {
					successCount++
					client.Logger.Info(fmt.Sprintf("Successfully deleted mergerules for %d repositories", len(repos)))
				}
			}

			if successCount == totalSections {
				client.Logger.Info(fmt.Sprintf("All %d sections deleted successfully for %d repositories", totalSections, len(repos)))
			} else if successCount > 0 {
				client.Logger.Warn(fmt.Sprintf("Deleted %d/%d sections successfully for %d repositories", successCount, totalSections, len(repos)))
			}

			return nil
		},
	}

	cmd.Flags().StringSliceVar(&deleteSections, "section", []string{}, "Sections to delete (repeatable or comma-separated): properties|reviewers|signatures|mergerules")
	cmd.Flags().StringVarP(&deleteRepoIdent, "repositorySlug", "s", "", "Repository identifier <projectKey>/<repoSlug> (optional when using --input)")
	cmd.Flags().StringVarP(&deleteInputPath, "input", "i", "", `Input YAML or JSON file or '-' for stdin with repositories
Example:
repositories:
  - projectKey: project_1
    repositorySlug: repo1
`)
	return cmd
}
