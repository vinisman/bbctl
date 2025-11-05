package workzone

import (
	"context"

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

			normalized, err := normalizeSections(deleteSections, false)
			if err != nil {
				return err
			}

			operations := map[string]sectionOperation{
				SectionProperties: {execute: wzClient.RemoveReposWorkflowProperties, message: "removed workflow properties"},
				SectionReviewers:  {execute: wzClient.DeleteReposReviewersList, message: "deleted reviewers"},
				SectionSignatures: {execute: wzClient.DeleteReposSignapprovers, message: "deleted sign approvers"},
				SectionMergerules: {execute: wzClient.DeleteReposAutomergers, message: "deleted mergerules"},
			}

			executeSections(client.Logger, repos, normalized, operations)
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
