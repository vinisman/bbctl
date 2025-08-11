package get

import (
	"github.com/spf13/cobra"
	"github.com/vinisman/bbctl/utils"
)

var (
	Columns        string
	ManifestFields string
	ManifestFile   string
	ManifestExist  bool
	OutputFormat   string
)

var GetCmd = &cobra.Command{
	Use:   "get",
	Short: "Get resources from Bitbucket",
}

func init() {
	GetCmd.PersistentFlags().StringVar(&Columns, "columns", "", `Extra columns to display (comma-separated)
Available columns:
 Name (default)
 Project (default)
 Description
 Archived (default)
 DefaultBranch
 Forkable
 Id
 Public
 Slug
 State (default)
	`)

	GetCmd.PersistentFlags().StringVar(&ManifestFile, "manifest-file", "", "Manifest JSON filename to read from each repo root")
	GetCmd.PersistentFlags().StringVar(&ManifestFields, "manifest-fields", "", "Comma-separated JSON fields to extract from manifest")
	GetCmd.PersistentFlags().BoolVar(&ManifestExist, "manifest-exist", false, "Show only repos where manifest file exists")
	GetCmd.PersistentFlags().StringVarP(&OutputFormat, "output", "o", "plain", "Output format: plain, yaml, json")
	GetCmd.PersistentFlags().StringVarP(&utils.ProjectKey, "project", "p", "", "Bitbucket project key (or env BITBUCKET_PROJECT_KEY)")

	GetCmd.AddCommand(getReposCmd)
	GetCmd.AddCommand(getRepoCmd)
	GetCmd.AddCommand(getRepoWebhooksCmd)
}
