package workzone

import (
	"context"
	"fmt"
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
			normalized := map[string]bool{}
			if len(sections) == 0 {
				normalized["properties"] = true
				normalized["reviewers"] = true
				normalized["signatures"] = true
				normalized["mergerules"] = true
			} else {
				for _, s := range sections {
					s = strings.TrimSpace(strings.ToLower(s))
					switch s {
					case "properties", "reviewers", "signatures", "mergerules":
						normalized[s] = true
					case "", "all":
						normalized["properties"] = true
						normalized["reviewers"] = true
						normalized["signatures"] = true
						normalized["mergerules"] = true
					default:
						return fmt.Errorf("unsupported --section: %s (supported: properties, reviewers, signatures, mergerules)", s)
					}
				}
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
			if normalized["properties"] {
				num++
				go func() {
					out, err := wzClient.GetRepoWorkflows(repos)
					resCh <- secRes{kind: "properties", out: out, err: err}
				}()
			}
			if normalized["reviewers"] {
				num++
				go func() {
					out, err := wzClient.GetReposReviewersList(repos)
					resCh <- secRes{kind: "reviewers", out: out, err: err}
				}()
			}
			if normalized["signatures"] {
				num++
				go func() {
					out, err := wzClient.GetReposSignapprovers(repos)
					resCh <- secRes{kind: "signatures", out: out, err: err}
				}()
			}
			if normalized["mergerules"] {
				num++
				go func() {
					out, err := wzClient.GetReposAutomergers(repos)
					resCh <- secRes{kind: "mergerules", out: out, err: err}
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
			if normalized["properties"] {
				fields = append(fields, "workzone.workflowProperties")
			}
			if normalized["reviewers"] {
				fields = append(fields, "workzone.reviewers")
			}
			if normalized["signatures"] {
				fields = append(fields, "workzone.signapprovers")
			}
			if normalized["mergerules"] {
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

// parseRepos is deprecated; use utils.ParseRepositoriesFromArgs instead

func mergeSection(base, add []models.ExtendedRepository, kind string) []models.ExtendedRepository {
	// index add by key/slug
	type key struct{ p, r string }
	addMap := make(map[key]models.ExtendedRepository, len(add))
	for _, it := range add {
		addMap[key{it.ProjectKey, it.RepositorySlug}] = it
	}
	out := make([]models.ExtendedRepository, len(base))
	copy(out, base)
	for i := range out {
		k := key{out[i].ProjectKey, out[i].RepositorySlug}
		if src, ok := addMap[k]; ok {
			if out[i].Workzone == nil {
				out[i].Workzone = &models.WorkzoneData{}
			}
			if src.Workzone != nil {
				switch kind {
				case "properties":
					out[i].Workzone.WorkflowProperties = src.Workzone.WorkflowProperties
				case "reviewers":
					out[i].Workzone.Reviewers = src.Workzone.Reviewers
				case "signatures":
					out[i].Workzone.Signapprovers = src.Workzone.Signapprovers
				case "mergerules":
					out[i].Workzone.Mergerules = src.Workzone.Mergerules
				}
			}
		}
	}
	return out
}
