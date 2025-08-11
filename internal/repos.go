package internal

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"

	"github.com/vinisman/bbctl/utils"
	"github.com/vinisman/bitbucket-sdk-go/openapi"
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
