package get

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"sync"
	"text/template"

	"github.com/spf13/cobra"
	"github.com/vinisman/bbctl/api"
	"github.com/vinisman/bbctl/models"
	"github.com/vinisman/bbctl/utils"
	"gopkg.in/yaml.v3"
)

var ReposCmd = &cobra.Command{
	Use:   "repos",
	Short: "Get list of repositories",
	Run: func(cmd *cobra.Command, args []string) {
		cfg := utils.LoadConfig()

		projectKey := utils.ProjectKey
		svc := api.NewRepoService(cfg, projectKey)
		summaries, err := svc.List()
		if err != nil {
			slog.Error("failed to list repos")
			slog.Debug("details", slog.Any("err", err))
			return
		}

		filters := parseFilter(utils.FilterExpr)
		manifests := map[string]map[string]interface{}{}
		includeBranch := strings.Contains(utils.FieldsToShow, "defaultBranch")
		includeTemplate := utils.TemplateStr != ""
		repos := make([]models.RepoEntity, 0, len(summaries))

		var wg sync.WaitGroup
		sem := make(chan struct{}, 5)

		for _, rs := range summaries {
			r := models.RepoEntity{
				Name:        rs.Name,
				Slug:        rs.Slug,
				Description: rs.Description,
				State:       rs.State,
				Public:      rs.Public,
				Forkable:    rs.Forkable,
				Archived:    rs.Archived,
			}

			wg.Add(1)
			sem <- struct{}{}
			go func(r models.RepoEntity) {
				defer wg.Done()
				defer func() { <-sem }()

				if includeBranch {
					r.DefaultBranch = *svc.GetDefaultBranch(r.Slug)
				}

				var manifest map[string]interface{}
				if includeTemplate || utils.FilterExpr != "" {
					manifest = tryLoadManifest(cfg, projectKey, r.Slug, utils.ManifestFile)
				}

				if manifest != nil {
					manifests[r.Slug] = manifest
				}

				if utils.FilterExpr != "" {
					if !matchesFilter(manifest, filters) {
						return
					}
				}

				if utils.ManifestExists {
					if manifest != nil {
						repos = append(repos, r)
					}
				} else {
					repos = append(repos, r)
				}
			}(r)
		}
		wg.Wait()

		if utils.OutputFile != "" {
			printRepoYaml(repos, utils.FieldsToShow, manifests)
		} else {
			printRepoFields(repos, utils.FieldsToShow, manifests)
		}
	},
}

func init() {
	ReposCmd.Flags().StringVar(&utils.FilterExpr, "filter", "", "Filter by manifest fields (e.g. 'type=library'). Works with manifest.json fields")
	ReposCmd.Flags().BoolVar(&utils.ManifestExists, "manifest-exists", false, "Prints only repositories where manifest file exists (e.g. tue/false)")
	ReposCmd.Flags().StringVar(&utils.ManifestFile, "manifest-file", "", "Manifest JSON filename in the root of repository (e.g. manifest.json)")
	ReposCmd.Flags().StringVar(&utils.TemplateStr, "template", "", "Go template using manifest fields (e.g. '{{ .type }}')")
	ReposCmd.Flags().StringVarP(&utils.OutputFile, "output", "o", "", "Output YAML file path")
	ReposCmd.Flags().StringVarP(&utils.FieldsToShow, "fields", "f", "name", `Comma-separated fields to show
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

func printRepoFields(repos []models.RepoEntity, fieldsCSV string, manifests map[string]map[string]interface{}) {
	fields := strings.Split(fieldsCSV, ",")
	templateField := ""
	var tmpl *template.Template
	if utils.TemplateStr != "" {
		trimmed := strings.Trim(utils.TemplateStr, "{} ")
		if strings.HasPrefix(trimmed, ".") {
			templateField = strings.TrimPrefix(trimmed, ".")
		}
		tmpl = template.Must(template.New("manifest").Parse(utils.TemplateStr))
	}

	header := append([]string{}, fields...)
	if templateField != "" {
		header = append(header, templateField)
	}
	fmt.Println(strings.Join(header, "\t"))

	for _, repo := range repos {
		values := []string{}
		manifest := manifests[repo.Slug]

		for _, f := range fields {
			switch strings.TrimSpace(f) {
			case "name":
				values = append(values, repo.Name)
			case "slug":
				values = append(values, repo.Slug)
			case "state":
				values = append(values, repo.State)
			case "public":
				values = append(values, fmt.Sprintf("%v", repo.Public))
			case "forkable":
				values = append(values, fmt.Sprintf("%v", repo.Forkable))
			case "archived":
				values = append(values, fmt.Sprintf("%v", repo.Archived))
			case "description":
				values = append(values, repo.Description)
			case "defaultBranch":
				values = append(values, repo.DefaultBranch)
			default:
				if manifest != nil {
					if val, ok := manifest[f]; ok {
						values = append(values, fmt.Sprintf("%v", val))
						continue
					}
				}
				values = append(values, "")
			}
		}

		if tmpl != nil {
			var sb strings.Builder
			_ = tmpl.Execute(&sb, manifest)
			values = append(values, sb.String())
		}

		fmt.Println(strings.Join(values, "\t"))
	}
}

func printRepoYaml(repos []models.RepoEntity, fieldsCSV string, manifests map[string]map[string]interface{}) {
	type outRepo map[string]interface{}
	var output []outRepo

	fields := strings.Split(fieldsCSV, ",")
	templateField := ""
	var tmpl *template.Template
	if utils.TemplateStr != "" {
		trimmed := strings.Trim(utils.TemplateStr, "{} ")
		if strings.HasPrefix(trimmed, ".") {
			templateField = strings.TrimPrefix(trimmed, ".")
		}
		var err error
		tmpl, err = template.New("tpl").Parse(utils.TemplateStr)
		if err != nil {
			slog.Debug("Invalid template", slog.String("error", err.Error()))
			return
		}
	}

	for _, r := range repos {
		entry := make(outRepo)
		entry["name"] = r.Name

		for _, f := range fields {
			f = strings.TrimSpace(f)
			switch f {
			case "slug":
				entry["slug"] = r.Slug
			case "state":
				entry["state"] = r.State
			case "public":
				entry["public"] = r.Public
			case "forkable":
				entry["forkable"] = r.Forkable
			case "archived":
				entry["archived"] = r.Archived
			case "description":
				entry["description"] = r.Description
			case "defaultBranch":
				entry["defaultBranch"] = r.DefaultBranch
			default:
				if manifest, ok := manifests[r.Slug]; ok {
					if val, ok := manifest[f]; ok {
						entry[f] = val
						continue
					}
				}
				entry[f] = ""
			}
		}

		if tmpl != nil && templateField != "" {
			var sb strings.Builder
			_ = tmpl.Execute(&sb, manifests[r.Slug])
			entry[templateField] = sb.String()
		}

		output = append(output, entry)
	}

	out := map[string]interface{}{"repositories": output}
	file, err := os.Create(utils.OutputFile)
	if err != nil {
		slog.Debug("Could not write output file", slog.String("error", err.Error()))
		return
	}
	defer file.Close()

	enc := yaml.NewEncoder(file)
	enc.SetIndent(2)
	if err := enc.Encode(out); err != nil {
		slog.Debug("YAML encode error", slog.String("error", err.Error()))
		return
	}

	slog.Info("Saved output", slog.String("path", utils.OutputFile))
}

func tryLoadManifest(cfg utils.Config, projectKey, repoSlug, manifestFile string) map[string]interface{} {
	log := slog.With("repo", repoSlug)
	if manifestFile == "" {
		return nil
	}
	url := fmt.Sprintf("%s/rest/api/latest/projects/%s/repos/%s/raw/%s", cfg.BitbucketURL, projectKey, repoSlug, manifestFile)
	req, _ := http.NewRequest("GET", url, nil)
	req.Header.Set("Authorization", "Bearer "+cfg.BitbucketToken)
	req.Header.Set("Accept", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil || resp.StatusCode != 200 {
		bodyBytes, _ := io.ReadAll(resp.Body)
		log.Error("failed to create", slog.Any("httpCode", resp.StatusCode), slog.String("response", string(bodyBytes)))
		return nil
	}
	defer resp.Body.Close()

	var data map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&data)
	return data
}

func parseFilter(filter string) map[string]string {
	m := make(map[string]string)
	if filter == "" {
		return m
	}
	pairs := strings.Split(filter, ",")
	for _, pair := range pairs {
		parts := strings.SplitN(pair, "=", 2)
		if len(parts) == 2 {
			m[strings.TrimSpace(parts[0])] = strings.TrimSpace(parts[1])
		}
	}
	return m
}

func matchesFilter(manifest map[string]interface{}, filters map[string]string) bool {
	for key, expected := range filters {
		if manifest == nil {
			return false
		}
		if val, ok := manifest[key]; !ok || fmt.Sprintf("%v", val) != expected {
			return false
		}
	}
	return true
}
