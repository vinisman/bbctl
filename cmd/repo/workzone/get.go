package workzone

import (
	"context"
	"strings"

	"github.com/spf13/cobra"
	bb "github.com/vinisman/bbctl/internal/bitbucket"
	"github.com/vinisman/bbctl/internal/models"
	wz "github.com/vinisman/bbctl/internal/workzone"
	"github.com/vinisman/bbctl/utils"
)

// get command flags
var (
	repoIdent string
	output    string
	sections  []string
	input     string
)

// GetWorkzoneCmd returns a cobra command to get workzone settings for a repository
func GetWorkzoneCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "get",
		Short: "Get Workzone settings for a repository",
		Long: `Fetch Workzone settings for one or more repositories.

Sections (plugin tab names):
  - properties: repository workflow properties
  - reviewers: branch reviewers list
  - signatures: branch sign approvers list
  - mergerules: branch automergers list

Selection rules:
  - If --section is not specified, all sections are fetched by default
  - You can specify multiple sections either by repeating --section or via comma-separated values

Examples:
  bbctl repo workzone get --repositorySlug KEY/slug
  bbctl repo workzone get --section reviewers,mergerules --repositorySlug KEY/slug -o yaml
  bbctl repo workzone get --section reviewers --section mergerules --input repos.yaml -o json`,
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := bb.NewClient(context.Background())
			if err != nil {
				return err
			}
			wzClient := wz.NewClient(client)

			repos, err := utils.ParseRepositoriesFromArgs(repoIdent, input)
			if err != nil {
				return err
			}

			// Normalize requested sections; default to all if none specified
			normalized, err := normalizeSections(sections, true)
			if err != nil {
				return err
			}

			// Execute selected fetches sequentially (each is internally concurrent per repo)
			// Run selected fetches in parallel and merge results
			agg := make([]models.ExtendedRepository, len(repos))
			copy(agg, repos)
			type secRes struct {
				kind string
				out  []models.ExtendedRepository
				err  error
			}
			resCh := make(chan secRes, 4)
			num := 0
			if normalized[SectionProperties] {
				num++
				go func() {
					out, err := wzClient.GetRepoWorkflows(repos)
					resCh <- secRes{kind: SectionProperties, out: out, err: err}
				}()
			}
			if normalized[SectionReviewers] {
				num++
				go func() {
					out, err := wzClient.GetReposReviewersList(repos)
					resCh <- secRes{kind: SectionReviewers, out: out, err: err}
				}()
			}
			if normalized[SectionSignatures] {
				num++
				go func() {
					out, err := wzClient.GetReposSignapprovers(repos)
					resCh <- secRes{kind: SectionSignatures, out: out, err: err}
				}()
			}
			if normalized[SectionMergerules] {
				num++
				go func() {
					out, err := wzClient.GetReposAutomergers(repos)
					resCh <- secRes{kind: SectionMergerules, out: out, err: err}
				}()
			}
			for i := 0; i < num; i++ {
				r := <-resCh
				if r.err != nil {
					client.Logger.Error(r.err.Error())
					continue
				}
				agg = mergeSection(agg, r.out, r.kind)
			}

			// Build output fields
			fields := []string{"projectKey", "repositorySlug"}
			if normalized[SectionProperties] {
				fields = append(fields, "workzone.workflowProperties")
			}
			if normalized[SectionReviewers] {
				fields = append(fields, "workzone.reviewers")
			}
			if normalized[SectionSignatures] {
				fields = append(fields, "workzone.signapprovers")
			}
			if normalized[SectionMergerules] {
				fields = append(fields, "workzone.mergerules")
			}
			return utils.PrintStructured("repositories", agg, output, strings.Join(fields, ","))
		},
	}

	cmd.Flags().StringVarP(&repoIdent, "repositorySlug", "s", "", "Repository identifier <projectKey>/<repoSlug>")
	cmd.Flags().StringVarP(&output, "output", "o", "yaml", "Output format: plain|yaml|json")
	cmd.Flags().StringSliceVar(&sections, "section", []string{}, "Sections to fetch (repeatable or comma-separated, default: all): properties|reviewers|signatures|mergerules")
	cmd.Flags().StringVarP(&input, "input", "i", "", `Input YAML or JSON file or '-' for stdin containing repositories
Example:
repositories:
  - projectKey: project_1
    repositorySlug: repo1
`)

	return cmd
}
