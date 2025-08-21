package repo

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
	"github.com/vinisman/bbctl/internal/bitbucket"
	"github.com/vinisman/bbctl/internal/models"
	"github.com/vinisman/bbctl/utils"
)

func NewGetCmd() *cobra.Command {
	var (
		projectKey     string
		repositorySlug string
		columns        string
		output         string
		showDetails    string
		manifestFile   string
		inputFile      string
	)

	cmd := &cobra.Command{
		Use:   "get",
		Short: "Get repositories from Bitbucket",
		RunE: func(cmd *cobra.Command, args []string) error {
			if (projectKey == "" && inputFile == "") || (projectKey != "" && inputFile != "") {
				return fmt.Errorf("please specify exactly one of --projectKey or --input")
			}

			client, err := bitbucket.NewClient(context.Background())
			if err != nil {
				return err
			}

			var repos []models.ExtendedRepository

			var cols []string

			options := models.RepositoryOptions{}

			if showDetails != "" && output != "plain" {
				// enable only the options specified in showDetails
				for _, opt := range utils.ParseColumns(showDetails) {
					switch opt {
					case "repository":
						options.Repository = true
					case "defaultBranch":
						options.DefaultBranch = true
					case "webhooks":
						options.Webhooks = true
					case "required-builds":
						options.RequiredBuilds = true
					case "manifest":
						if manifestFile == "" {
							return fmt.Errorf("please specify --manifest-file")
						}
						options.Manifest = true
						options.ManifestPath = &manifestFile
					}
				}
			} else {
				// default options
				options = models.RepositoryOptions{
					Repository:     true,
					Webhooks:       false,
					DefaultBranch:  false,
					Manifest:       false,
					RequiredBuilds: false,
				}
				if utils.HasOption(columns, "defaultBranch") {
					options.DefaultBranch = true
				}
			}

			if inputFile != "" {
				var parsed struct {
					Repositories []models.ExtendedRepository `yaml:"repositories"`
				}
				if err := utils.ParseYAMLFile(inputFile, &parsed); err != nil {
					return err
				}
				for _, repo := range parsed.Repositories {
					r, err := client.GetReposBySlugs(repo.ProjectKey, []string{repo.RepositorySlug}, options)
					if err != nil {
						return err
					}
					repos = append(repos, r...)
				}
			} else {
				projects := utils.ParseColumns(projectKey)
				slugList := utils.ParseColumns(repositorySlug)

				if columns != "" {
					cols = utils.ParseColumns(columns)
				} else {
					// default columns for repositories
					cols = []string{"Id", "Name", "Project"}
				}

				switch {
				case len(slugList) > 0 && len(projects) == 1:
					// Get repos by project & slug(s)
					repos, err = client.GetReposBySlugs(projects[0], slugList, options)
					if err != nil {
						return err
					}

				case len(slugList) == 0 && len(projects) == 1:
					// Get all repos for single project
					repos, err = client.GetAllReposForProject(projects[0], options)
					if err != nil {
						return err
					}
				case len(slugList) == 0 && len(projects) > 1:
					// Get all repos for multiple projects
					repos, err = client.GetAllRepos(projects, options)
					if err != nil {
						return err
					}
				default:
					return fmt.Errorf("invalid combination of --project and --slug")
				}
			}

			// Structured output
			if output == "yaml" || output == "json" {
				return utils.PrintStructured("repositories", repos, output, columns)
			}
			utils.PrintRepos(repos, cols)

			return nil
		},
	}

	cmd.Flags().StringVarP(&projectKey, "projectKey", "k", "", "Comma-separated project keys")
	cmd.Flags().StringVarP(&repositorySlug, "repositorySlug", "s", "", "Comma-separated repository slugs (requires single project)")
	cmd.Flags().StringVar(&columns, "columns", "", "Comma-separated list of fields to display (for plain output)")
	cmd.Flags().StringVarP(&output, "output", "o", "plain", "Output format: plain|yaml|json")
	cmd.Flags().StringVar(&manifestFile, "manifest-file", "", "Path to the manifest file to output")
	cmd.Flags().StringVar(&showDetails, "show-details", "repository", `Comma-separated list of options to include in YAML/JSON output
	Supported:
	  repository
	  webhooks
	  defaultbranch
	  webhooks
	  manifest
	  required-builds
	`)
	cmd.Flags().StringVarP(&inputFile, "input", "i", "", "Path to input YAML file containing repositories")

	return cmd
}
