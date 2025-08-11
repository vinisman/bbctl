package internal

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"sync"
	"text/tabwriter"

	"github.com/vinisman/bbctl/utils"
	"github.com/vinisman/bitbucket-sdk-go/openapi"
	"gopkg.in/yaml.v2"
)

func FetchDefaultBranches(client *openapi.APIClient, repos []openapi.RestRepository) {

	workerCount := utils.WorkerCount

	if len(repos) < workerCount {
		workerCount = len(repos)
	}

	type job struct {
		repo *openapi.RestRepository
	}

	jobs := make(chan job)
	var wg sync.WaitGroup

	for i := 0; i < workerCount; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := range jobs {
				resp, httpResp, err := client.ProjectAPI.
					GetDefaultBranch2(context.Background(), utils.DerefString(&j.repo.Project.Key), utils.DerefString(j.repo.Slug)).
					Execute()
				if err != nil {
					utils.Logger.Debug("Unable to get default branch", "repo", j.repo.Slug, "error", err.Error(), "httpResp", httpResp)
					continue
				}
				if resp.DisplayId != nil {
					j.repo.DefaultBranch = resp.DisplayId
				}
			}
		}()
	}

	for i := range repos {
		jobs <- job{repo: &repos[i]}
	}

	close(jobs)
	wg.Wait()
}

func PrintRepos(repos []openapi.RestRepository, extraCols string, manifestFields []string, manifestData map[string]map[string]string, format string) {
	defaultCols := []string{"Name", "Archived", "State", "Project"}
	var allCols []string
	if extraCols != "" {
		extra := utils.ParseColumns(extraCols)
		allCols = extra
	} else {
		allCols = defaultCols
	}
	if len(manifestFields) > 0 {
		for _, mf := range manifestFields {
			allCols = append(allCols, "m_"+mf)
		}
	}

	type repoRow map[string]interface{}
	var rows []repoRow

	for _, repo := range repos {
		row := repoRow{}
		for _, col := range allCols {
			key := strings.TrimSpace(col)
			var val interface{}

			switch key {
			case "Id":
				val = utils.DerefInt32(repo.Id)
			case "Name":
				val = utils.DerefString(repo.Name)
			case "Project":
				val = utils.DerefString(repo.Project.Name)
			case "State":
				val = utils.DerefString(repo.State)
			case "Archived":
				val = utils.DerefBool(repo.Archived)
			case "DefaultBranch":
				val = utils.DerefString(repo.DefaultBranch)
			case "Forkable":
				val = utils.DerefBool(repo.Forkable)
			case "Slug":
				val = utils.DerefString(repo.Slug)
			case "ScmId":
				val = utils.DerefString(repo.ScmId)
			case "Description":
				val = utils.DerefString(repo.Description)
			case "Public":
				val = utils.DerefBool(repo.Public)
			default:
				if strings.HasPrefix(key, "m_") && manifestData != nil {
					fieldName := strings.TrimPrefix(key, "m_")
					slug := utils.DerefString(repo.Slug)
					if m, ok := manifestData[slug]; ok {
						val = m[fieldName]
					} else {
						val = ""
					}
				} else {
					val = ""
				}
			}

			// first letter to lowercase
			if format == "json" || format == "yaml" {
				lowerKey := strings.ToLower(key[:1]) + key[1:]
				row[lowerKey] = val
			} else {
				row[key] = val
			}
		}
		rows = append(rows, row)
	}

	wrapped := map[string]interface{}{
		"repos": rows,
	}

	switch strings.ToLower(format) {
	case "yaml":
		out, err := yaml.Marshal(wrapped)
		if err != nil {
			log.Fatalf("Error generating YAML: %v", err)
		}
		fmt.Print(string(out))
	case "json":
		out, err := json.MarshalIndent(wrapped, "", "  ")
		if err != nil {
			log.Fatalf("Error generating JSON: %v", err)
		}
		fmt.Print(string(out))
	default: // plain
		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)

		// Headers to uppercase
		var header []string
		for _, col := range allCols {
			header = append(header, strings.ToUpper(col))
		}
		fmt.Fprintln(w, strings.Join(header, "\t"))

		// values
		for _, row := range rows {
			var vals []string
			for _, col := range allCols {
				vals = append(vals, fmt.Sprintf("%v", row[col]))
			}
			fmt.Fprintln(w, strings.Join(vals, "\t"))
		}
		w.Flush()
	}
}

func downloadFileFromRepo(client *openapi.APIClient, projectKey, repoSlug, filePath string) ([]byte, error) {

	baseURL := client.GetConfig().Servers[0].URL
	url := fmt.Sprintf("%s/api/1.0/projects/%s/repos/%s/raw/%s", baseURL, projectKey, repoSlug, filePath)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}

	for k, v := range client.GetConfig().DefaultHeader {
		req.Header.Set(k, v)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	return io.ReadAll(resp.Body)
}

func parseManifestFields(content []byte, keys []string) map[string]string {
	result := make(map[string]string)
	var data map[string]interface{}
	if err := json.Unmarshal(content, &data); err != nil {
		utils.Logger.Error("Error parsing manifest file", "err", err)
		return result
	}
	var missingKeys []string
	for _, key := range keys {
		if v, ok := data[key]; ok {
			switch val := v.(type) {
			case string:
				result[key] = val
			default:
				result[key] = fmt.Sprintf("%v", val)
			}
		} else {
			result[key] = ""
			missingKeys = append(missingKeys, key)
		}
	}

	if len(missingKeys) > 0 {
		utils.Logger.Debug("Manifest keys not found", "keys", missingKeys)
	}
	return result
}

func FetchManifests(client *openapi.APIClient, repos []openapi.RestRepository, manifestFile string, fields []string) map[string]map[string]string {
	workerCount := utils.WorkerCount
	if len(repos) < workerCount {
		workerCount = len(repos)
	}

	type job struct {
		repo openapi.RestRepository
	}

	type result struct {
		slug   string
		values map[string]string
	}

	jobs := make(chan job)
	results := make(chan result)

	var wg sync.WaitGroup

	for i := 0; i < workerCount; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := range jobs {
				slug := utils.DerefString(j.repo.Slug)
				projectKey := utils.DerefString(&j.repo.Project.Key)
				content, err := downloadFileFromRepo(client, projectKey, slug, manifestFile)
				if err != nil {
					utils.Logger.Debug("Unable to get manifest file", "repo", slug, "file", manifestFile, "error", err.Error())
					continue
				}
				values := parseManifestFields(content, fields)
				results <- result{slug: slug, values: values}
			}
		}()
	}

	go func() {
		wg.Wait()
		close(results)
	}()

	go func() {
		for _, r := range repos {
			jobs <- job{repo: r}
		}
		close(jobs)
	}()

	manifestDataMap := make(map[string]map[string]string)

	for res := range results {
		manifestDataMap[res.slug] = res.values
	}

	return manifestDataMap
}
