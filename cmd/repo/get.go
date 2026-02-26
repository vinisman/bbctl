package repo

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"

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
		configFiles    []string
		input          string
	)

	cmd := &cobra.Command{
		Use:   "get",
		Short: "Get repositories from Bitbucket",
		Long: `Get repositories from Bitbucket.
You can specify either:
  --projectKey to get all repositories for one or more projects
  --repositorySlug to get a specific repository
  --input to load repository identifiers from a YAML file
Only one of these options should be used at a time.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			parseConfigFiles := func(items []string) (map[string]string, error) {
				result := make(map[string]string)
				for _, item := range items {
					item = strings.TrimSpace(item)
					if item == "" {
						continue
					}
					parts := strings.SplitN(item, "=", 2)
					if len(parts) != 2 {
						return nil, fmt.Errorf("invalid config file format: %q (expected key=filepath)", item)
					}
					key := strings.TrimSpace(parts[0])
					filepath := strings.TrimSpace(parts[1])
					if key == "" || filepath == "" {
						return nil, fmt.Errorf("key and filepath must not be empty: %q", item)
					}
					result[key] = filepath
				}
				return result, nil
			}

			configFileMap, err := parseConfigFiles(configFiles)
			if err != nil {
				return err
			}

			count := 0
			if projectKey != "" {
				count++
			}
			if repositorySlug != "" {
				count++
			}
			if input != "" {
				count++
			}
			if count != 1 {
				return fmt.Errorf("please specify exactly one of --projectKey, --repositorySlug or --input")
			}

			client, err := bitbucket.NewClient(context.Background())
			if err != nil {
				return err
			}

			var repos []models.ExtendedRepository

			var cols []string

			options := models.RepositoryOptions{}
			// Track exactly what user requested to control output stripping
			requestedRepository := false
			requestedWebhooks := false
			requestedRequiredBuilds := false
			requestedManifest := false
			requestedConfigs := false

			// Validate: if user explicitly set --show-details to empty -> error
			if cmd.Flags().Changed("show-details") && strings.TrimSpace(showDetails) == "" {
				return fmt.Errorf("--show-details cannot be empty")
			}

			if showDetails != "" && output != "plain" {
				// enable only the options specified in showDetails
				for _, opt := range utils.ParseColumnsToLower(showDetails) {
					switch opt {
					case "repository":
						options.Repository = true
						requestedRepository = true
					case "defaultbranch":
						options.DefaultBranch = true
					case "webhooks":
						options.Webhooks = true
						requestedWebhooks = true
					case "required-builds":
						options.RequiredBuilds = true
						requestedRequiredBuilds = true
					case "manifest":
						if manifestFile == "" {
							return fmt.Errorf("please specify --manifest-file")
						}
						options.Manifest = true
						options.ManifestPath = &manifestFile
						requestedManifest = true
					case "configs":
						if len(configFileMap) == 0 {
							return fmt.Errorf("please specify --config-file")
						}
						options.ConfigFiles = true
						options.ConfigFileMap = configFileMap
						requestedConfigs = true
					}
				}

				if !cmd.Flags().Changed("show-details") && len(configFileMap) > 0 {
					options.ConfigFiles = true
					options.ConfigFileMap = configFileMap
					requestedConfigs = true
				}

				// Ensure repository details are fetched when defaultBranch is requested
				if options.DefaultBranch {
					options.Repository = true
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

			if input != "" {
				var parsed models.RepositoryYaml
				if err := utils.ParseFile(input, &parsed); err != nil {
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
				projects := utils.ParseColumnsToLower(projectKey)
				slugList := utils.ParseColumnsToLower(repositorySlug)

				if columns != "" {
					cols = utils.ParseColumnsToLower(columns)
				} else {
					// default columns for repositories
					cols = []string{"Id", "Name", "Slug", "Project"}
				}

				if len(slugList) > 0 {
					projectMap := make(map[string][]string)
					for _, combined := range slugList {
						parts := strings.SplitN(combined, "/", 2)
						if len(parts) != 2 || parts[1] == "" {
							client.Logger.Error("invalid repository identifier format", slog.String("identifier", combined))
							continue
						}
						project := parts[0]
						slug := parts[1]
						projectMap[project] = append(projectMap[project], slug)
					}

					for project, slugs := range projectMap {
						r, err := client.GetReposBySlugs(project, slugs, options)
						if err != nil {
							return err
						}
						repos = append(repos, r...)
					}
				} else if len(slugList) == 0 && len(projects) == 1 {
					// Get all repos for single project
					repos, err = client.GetAllReposForProject(projects[0], options)
					if err != nil {
						return err
					}
				} else if len(slugList) == 0 && len(projects) > 1 {
					// Get all repos for multiple projects
					repos, err = client.GetAllRepos(projects, options)
					if err != nil {
						return err
					}
				} else {
					return fmt.Errorf("invalid combination of --projectKey and --repositorySlug")
				}
			}

			// If user explicitly provided show-details, strip fields not requested before structured output
			if showDetails != "" && output != "plain" {
				for i := range repos {
					if !requestedRepository {
						repos[i].RestRepository = nil
					}
					if !requestedWebhooks {
						repos[i].Webhooks = nil
					}
					if !requestedRequiredBuilds {
						repos[i].RequiredBuilds = nil
					}
					if !requestedManifest {
						repos[i].Manifest = nil
					}
					if !requestedConfigs {
						repos[i].ConfigFiles = nil
					}
					// DefaultBranch is only populated when requested; no action needed here
				}
			}

			// Structured output
			if output == "yaml" || output == "json" {
				if requestedConfigs {
					outRepos := make([]map[string]any, 0, len(repos))
					for _, repo := range repos {
						payload, err := json.Marshal(repo)
						if err != nil {
							return err
						}
						item := make(map[string]any)
						if err := json.Unmarshal(payload, &item); err != nil {
							return err
						}
						if repo.ConfigFiles != nil {
							for key, value := range *repo.ConfigFiles {
								item[key] = value
							}
						}
						delete(item, "configFiles")
						outRepos = append(outRepos, item)
					}
					return utils.PrintStructured("repositories", outRepos, output, columns)
				}
				return utils.PrintStructured("repositories", repos, output, columns)
			}
			utils.PrintRepos(repos, cols)

			return nil
		},
	}

	cmd.Flags().StringVarP(&projectKey, "projectKey", "k", "", "Comma-separated project keys")
	cmd.Flags().StringVarP(&repositorySlug, "repositorySlug", "s", "", "Comma-separated repository identifiers in format <projectKey>/<repositorySlug>")
	cmd.Flags().StringVar(&columns, "columns", "", "Comma-separated list of fields to display (for plain output)")
	cmd.Flags().StringVarP(&output, "output", "o", "plain", "Output format: plain|yaml|json")
	cmd.Flags().StringVar(&manifestFile, "manifest-file", "", "Path to the manifest file to output")
	cmd.Flags().StringSliceVar(&configFiles, "config-file", []string{}, "Config file(s) to output as separate sections in format key=filepath (repeat flag or use comma-separated values)")
	cmd.Flags().StringVar(&showDetails, "show-details", "repository", `Comma-separated list of options to include in YAML/JSON output
	Supported:
	  repository
	  defaultBranch
	  manifest
	  configs
	  webhooks
	  required-builds
	`)
	cmd.Flags().StringVarP(&input, "input", "i", "", "Path to input YAML or JSON file containing repositories (use '-' to read from stdin)")

	return cmd
}
