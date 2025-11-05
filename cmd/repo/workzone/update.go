package workzone

import (
	"context"
	"fmt"

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

			normalized, err := normalizeSections(updateSections, false)
			if err != nil {
				return err
			}

			operations := map[string]sectionOperation{
				SectionProperties: {execute: wzClient.UpdateReposWorkflowProperties, message: "updated workflow properties"},
				SectionReviewers:  {execute: wzClient.SetReposReviewersList, message: "updated reviewers"},
				SectionSignatures: {execute: wzClient.SetReposSignapprovers, message: "updated sign approvers"},
				SectionMergerules: {execute: wzClient.SetReposAutomergers, message: "updated mergerules"},
			}

			executeSections(client.Logger, repos, normalized, operations)
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
