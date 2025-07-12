// cmd/get/repo.go
package get

import (
	"log/slog"
	"strings"

	"github.com/spf13/cobra"
	"github.com/vinisman/bbctl/api"
	"github.com/vinisman/bbctl/models"
	"github.com/vinisman/bbctl/utils"
)

var RepoCmd = &cobra.Command{
	Use:   "repo <slug>",
	Short: "Get detailed info for a single repository",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		cfg := utils.LoadConfig()
		pk := utils.ProjectKey
		repoSlug := args[0]
		includeBranch := strings.Contains(utils.FieldsToShow, "defaultBranch")
		svc := api.NewRepoService(cfg, pk)
		rs, err := svc.Get(repoSlug)
		if err != nil {
			slog.Error("Failed to get repo")
			return
		}

		manifest := map[string]interface{}{}
		if utils.ManifestFile != "" {
			manifest = tryLoadManifest(cfg, pk, repoSlug, utils.ManifestFile)
		}

		repos := []models.RepoEntity{{
			Name:          rs.Name,
			Slug:          rs.Slug,
			Description:   rs.Description,
			State:         rs.State,
			Public:        rs.Public,
			Forkable:      rs.Forkable,
			Archived:      rs.Archived,
			DefaultBranch: rs.DefaultBranch,
		}}

		if includeBranch {
			repos[0].DefaultBranch = *svc.GetDefaultBranch(rs.Slug)
		}

		outMan := map[string]map[string]interface{}{rs.Slug: manifest}

		if utils.OutputFile != "" {
			printRepoYaml(repos, utils.FieldsToShow, outMan)
		} else {
			printRepoFields(repos, utils.FieldsToShow, outMan)
		}
	},
}

func init() {
	RepoCmd.Flags().StringVar(&utils.FilterExpr, "filter", "", "Filter by manifest fields (e.g. 'type=library'). Works with manifest.json fields")
	RepoCmd.Flags().StringVar(&utils.ManifestFile, "manifest-file", "", "Manifest JSON filename in the root of repository (e.g. manifest.json)")
	RepoCmd.Flags().StringVar(&utils.TemplateStr, "template", "", "Go template using manifest fields, e.g. '{{ .type }}'")
	RepoCmd.Flags().StringVarP(&utils.OutputFile, "output", "o", "", "Output YAML file")
	RepoCmd.Flags().StringVarP(&utils.FieldsToShow, "fields", "f", "name", `Comma-separated fields to show
	Available fields:
		slug
		state
		public
		forkable
		archived
		description
		defaultBranch
	`)
}
