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

			normalized, err := normalizeSections(setSections, false)
			if err != nil {
				return err
			}

			operations := map[string]sectionOperation{
				SectionProperties: {execute: wzClient.SetReposWorkflowProperties, message: "set workflow properties"},
				SectionReviewers:  {execute: wzClient.SetReposReviewersList, message: "set reviewers"},
				SectionSignatures: {execute: wzClient.SetReposSignapprovers, message: "set sign approvers"},
				SectionMergerules: {execute: wzClient.SetReposAutomergers, message: "set mergerules"},
			}

			executeSections(client.Logger, repos, normalized, operations)
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
