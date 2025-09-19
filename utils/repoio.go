package utils

import (
	"encoding/json"
	"os"
	"strings"

	"github.com/vinisman/bbctl/internal/models"
	openapi "github.com/vinisman/bitbucket-sdk-go/openapi"
	"gopkg.in/yaml.v3"
)

// WriteRepositoriesToFile writes repositories grouped by projectKey/repositorySlug to a file.
// The payload is wrapped under key "repositories". Format is controlled by the `format` argument (json|yaml).
// For each repository, both RequiredBuilds and Webhooks are merged across duplicates and de-duplicated by id.
func WriteRepositoriesToFile(path string, repos []models.ExtendedRepository, format string) error {
	grouped := GroupRepositories(repos)
	wrapper := map[string]interface{}{"repositories": grouped}
	var data []byte
	var err error
	switch strings.ToLower(format) {
	case "yaml", "yml":
		data, err = yaml.Marshal(wrapper)
	default:
		data, err = json.MarshalIndent(wrapper, "", "  ")
	}
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}

// GroupRepositories merges items for the same repo and deduplicates both required-builds and webhooks by id.
func GroupRepositories(repos []models.ExtendedRepository) []models.ExtendedRepository {
	repoMap := make(map[string]*models.ExtendedRepository)
	order := make([]string, 0)

	for _, r := range repos {
		key := r.ProjectKey + "/" + r.RepositorySlug
		entry, exists := repoMap[key]
		if !exists {
			copy := models.ExtendedRepository{ProjectKey: r.ProjectKey, RepositorySlug: r.RepositorySlug}
			// initialize empty slices for present kinds to avoid nil checks later
			if r.RequiredBuilds != nil {
				emptyRB := []openapi.RestRequiredBuildCondition{}
				copy.RequiredBuilds = &emptyRB
			}
			if r.Webhooks != nil {
				emptyWH := []openapi.RestWebhook{}
				copy.Webhooks = &emptyWH
			}
			repoMap[key] = &copy
			order = append(order, key)
			entry = &copy
		}
		// merge required builds
		if r.RequiredBuilds != nil && len(*r.RequiredBuilds) > 0 {
			if entry.RequiredBuilds == nil {
				empty := []openapi.RestRequiredBuildCondition{}
				entry.RequiredBuilds = &empty
			}
			*entry.RequiredBuilds = append(*entry.RequiredBuilds, (*r.RequiredBuilds)...)
		}
		// merge webhooks
		if r.Webhooks != nil && len(*r.Webhooks) > 0 {
			if entry.Webhooks == nil {
				empty := []openapi.RestWebhook{}
				entry.Webhooks = &empty
			}
			*entry.Webhooks = append(*entry.Webhooks, (*r.Webhooks)...)
		}
	}

	// Deduplicate per repo
	for _, key := range order {
		entry := repoMap[key]
		// dedup required builds by id (int64)
		if entry.RequiredBuilds != nil && len(*entry.RequiredBuilds) > 0 {
			seen := make(map[int64]bool)
			dedup := make([]openapi.RestRequiredBuildCondition, 0, len(*entry.RequiredBuilds))
			for _, rb := range *entry.RequiredBuilds {
				if rb.Id != nil {
					if seen[*rb.Id] {
						continue
					}
					seen[*rb.Id] = true
				}
				dedup = append(dedup, rb)
			}
			*entry.RequiredBuilds = dedup
		}
		// dedup webhooks by id (int32)
		if entry.Webhooks != nil && len(*entry.Webhooks) > 0 {
			seen := make(map[int32]bool)
			dedup := make([]openapi.RestWebhook, 0, len(*entry.Webhooks))
			for _, wh := range *entry.Webhooks {
				if wh.Id != nil {
					if seen[*wh.Id] {
						continue
					}
					seen[*wh.Id] = true
				}
				dedup = append(dedup, wh)
			}
			*entry.Webhooks = dedup
		}
	}

	out := make([]models.ExtendedRepository, 0, len(order))
	for _, key := range order {
		out = append(out, *repoMap[key])
	}
	return out
}
