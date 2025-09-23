package bitbucket

import (
	"fmt"
	"strings"
	"sync"

	"github.com/vinisman/bbctl/internal/config"
	"github.com/vinisman/bbctl/internal/models"
	"github.com/vinisman/bbctl/utils"
	openapi "github.com/vinisman/bitbucket-sdk-go/openapi"
)

// CreateRequiredBuilds creates required build merge checks for multiple repositories in parallel
func (c *Client) CreateRequiredBuilds(repos []models.ExtendedRepository) ([]models.ExtendedRepository, error) {
	maxWorkers := config.GlobalMaxWorkers

	type job struct {
		repo models.ExtendedRepository
		req  openapi.RestRequiredBuildConditionSetRequest
	}

	// count the total number
	var total int
	for _, r := range repos {
		total += len(*r.RequiredBuilds)
	}

	jobs := make(chan job, total)
	errCh := make(chan error, total)

	var wg sync.WaitGroup

	createdBuildsMu := sync.Mutex{}
	createdBuildsMap := make(map[string][]openapi.RestRequiredBuildCondition)

	worker := func() {
		defer wg.Done()
		for j := range jobs {
			created, httpResp, err := c.api.BuildsAndDeploymentsAPI.
				CreateRequiredBuildsMergeCheck(c.authCtx, j.repo.ProjectKey, j.repo.RepositorySlug).
				RestRequiredBuildConditionSetRequest(j.req).
				Execute()

			if err != nil && httpResp != nil {
				c.logger.Debug("HTTP response", "status", httpResp.StatusCode, "body", httpResp.Body)
				c.logger.Error("Failed to create required-build",
					"project", j.repo.ProjectKey,
					"slug", j.repo.RepositorySlug,
					"buildParentKeys", j.req.BuildParentKeys,
					"error", err)
				errCh <- err
				continue
			}

			// Log with build key/ID from returned RestRequiredBuildCondition
			var buildKey interface{}
			if created != nil {
				if created.Id != nil {
					buildKey = *created.Id
				} else if len(created.BuildParentKeys) > 0 {
					buildKey = created.BuildParentKeys
				}
			}
			c.logger.Info("Created required build merge check",
				"project", j.repo.ProjectKey,
				"slug", j.repo.RepositorySlug,
				"buildKey", utils.SafeInterface(buildKey))

			// Only add if Id is not nil
			if created != nil && created.Id != nil {
				createdBuildsMu.Lock()
				createdBuildsMap[j.repo.ProjectKey+"/"+j.repo.RepositorySlug] = append(createdBuildsMap[j.repo.ProjectKey+"/"+j.repo.RepositorySlug], *created)
				createdBuildsMu.Unlock()
			}
		}
	}

	// start worker pool
	wg.Add(maxWorkers)
	for range maxWorkers {
		go worker()
	}

	// send all jobs to the channel
	for _, r := range repos {
		for _, wh := range *r.RequiredBuilds {
			req := toSetRequest(wh)
			jobs <- job{repo: r, req: req}
		}
	}
	close(jobs)

	wg.Wait()
	close(errCh)

	// collect all errors
	var firstErr error
	for e := range errCh {
		if firstErr == nil {
			firstErr = e
		}
	}

	// build slice of repos with created builds
	var createdRepos []models.ExtendedRepository
	for _, r := range repos {
		key := r.ProjectKey + "/" + r.RepositorySlug
		if builds, ok := createdBuildsMap[key]; ok && len(builds) > 0 {
			// Assign only the created builds
			r.RequiredBuilds = &builds
			createdRepos = append(createdRepos, r)
		}
	}

	return createdRepos, nil
}

func (c *Client) UpdateRequiredBuilds(repos []models.ExtendedRepository) ([]models.ExtendedRepository, error) {
	maxWorkers := config.GlobalMaxWorkers

	updatedRepos := make([]models.ExtendedRepository, len(repos))
	for i := range repos {
		updatedRepos[i].ProjectKey = repos[i].ProjectKey
		updatedRepos[i].RepositorySlug = repos[i].RepositorySlug
		updatedRepos[i].RequiredBuilds = &[]openapi.RestRequiredBuildCondition{}
	}

	type job struct {
		repoIndex int
		repo      models.ExtendedRepository
		req       openapi.RestRequiredBuildConditionSetRequest
		id        int64
	}

	// count the total number
	var total int
	for _, r := range repos {
		total += len(*r.RequiredBuilds)
	}

	jobs := make(chan job, total)
	errCh := make(chan error, total)

	var wg sync.WaitGroup
	var mu sync.Mutex

	worker := func() {
		defer wg.Done()
		for j := range jobs {
			updated, httpResp, err := c.api.BuildsAndDeploymentsAPI.
				UpdateRequiredBuildsMergeCheck(c.authCtx, j.repo.ProjectKey, j.id, j.repo.RepositorySlug).
				RestRequiredBuildConditionSetRequest(j.req).
				Execute()

			if err != nil && httpResp != nil {
				c.logger.Debug("HTTP response", "status", httpResp.StatusCode, "body", httpResp.Body)
				if httpResp.StatusCode == 404 {
					// Skip missing items: treat as no-op
					c.logger.Info("Required build not found during update, skipping",
						"project", j.repo.ProjectKey,
						"slug", j.repo.RepositorySlug,
						"id", j.id)
					continue
				}
				errCh <- fmt.Errorf("failed to update required-build %v in %s/%s: %w",
					j.req.BuildParentKeys,
					j.repo.ProjectKey, j.repo.RepositorySlug, err)
				continue
			}

			// Log with build key/ID from returned RestRequiredBuildCondition
			var buildKey interface{}
			if updated != nil {
				if updated.Id != nil {
					buildKey = *updated.Id
				} else if len(updated.BuildParentKeys) > 0 {
					buildKey = updated.BuildParentKeys
				}
			}
			c.logger.Info("Updated required build merge check",
				"project", j.repo.ProjectKey,
				"slug", j.repo.RepositorySlug,
				"buildKey", buildKey)

			if updated != nil {
				mu.Lock()
				*updatedRepos[j.repoIndex].RequiredBuilds = append(*updatedRepos[j.repoIndex].RequiredBuilds, *updated)
				mu.Unlock()
			}
		}
	}

	// start worker pool
	wg.Add(maxWorkers)
	for range maxWorkers {
		go worker()
	}

	// send all jobs to the channel
	for i, r := range repos {
		for _, wh := range *r.RequiredBuilds {
			if wh.Id != nil {
				req := toSetRequest(wh)
				jobs <- job{repoIndex: i, repo: r, req: req, id: *wh.Id}
			}
		}
	}
	close(jobs)

	wg.Wait()
	close(errCh)

	// collect first error only
	var firstErr error
	for e := range errCh {
		if firstErr == nil {
			firstErr = e
		}
	}

	// filter updatedRepos to only those with len(*RequiredBuilds) > 0
	var filteredRepos []models.ExtendedRepository
	for _, r := range updatedRepos {
		if r.RequiredBuilds != nil && len(*r.RequiredBuilds) > 0 {
			filteredRepos = append(filteredRepos, r)
		}
	}

	return filteredRepos, firstErr
}

func (c *Client) DeleteRequiredBuilds(repos []models.ExtendedRepository) error {
	maxWorkers := config.GlobalMaxWorkers

	type job struct {
		repo models.ExtendedRepository
		id   int64
	}

	// count the total number
	var total int
	for _, r := range repos {
		total += len(*r.RequiredBuilds)
	}

	jobs := make(chan job, total)
	errCh := make(chan error, total)

	var wg sync.WaitGroup

	worker := func() {
		defer wg.Done()
		for j := range jobs {
			httpResp, err := c.api.BuildsAndDeploymentsAPI.
				DeleteRequiredBuildsMergeCheck(c.authCtx, j.repo.ProjectKey, j.id, j.repo.RepositorySlug).
				Execute()

			if err != nil {
				if httpResp != nil {
					c.logger.Debug("HTTP response", "status", httpResp.StatusCode, "body", httpResp.Body)
					if httpResp.StatusCode == 404 {
						// Skip missing items: treat as no-op
						c.logger.Info("Required build was already absent (404), skipping",
							"project", j.repo.ProjectKey,
							"slug", j.repo.RepositorySlug,
							"buildId", j.id)
						continue
					}
				}
				c.logger.Error("Failed to delete required build merge check",
					"project", j.repo.ProjectKey,
					"slug", j.repo.RepositorySlug,
					"buildId", j.id,
					"error", err)
				errCh <- err
				continue
			}

			c.logger.Info("Required builds was successfully deleted, or was never present",
				"project", j.repo.ProjectKey,
				"slug", j.repo.RepositorySlug,
				"buildId", j.id)
		}
	}

	// start worker pool
	wg.Add(maxWorkers)
	for range maxWorkers {
		go worker()
	}

	// send all jobs to the channel
	for _, r := range repos {
		for _, wh := range *r.RequiredBuilds {
			if wh.Id != nil {
				jobs <- job{repo: r, id: *wh.Id}
			}
		}
	}
	close(jobs)

	wg.Wait()
	close(errCh)

	// collect all errors
	var errs []string
	for e := range errCh {
		errs = append(errs, e.Error())
	}
	if len(errs) > 0 {
		return fmt.Errorf("errors occurred deleting required builds")
	}

	return nil
}

// helper to convert RestRequiredBuildCondition -> RestRequiredBuildConditionSetRequest
func toSetRequest(cond openapi.RestRequiredBuildCondition) openapi.RestRequiredBuildConditionSetRequest {
	req := openapi.RestRequiredBuildConditionSetRequest{
		BuildParentKeys:  cond.BuildParentKeys,
		ExemptRefMatcher: cond.ExemptRefMatcher,
	}
	if cond.RefMatcher != nil {
		req.RefMatcher = openapi.RestRefMatcher{
			Id:        cond.RefMatcher.Id,
			DisplayId: cond.RefMatcher.DisplayId,
			Type:      cond.RefMatcher.Type,
		}
	}
	return req
}

func (c *Client) GetRequiredBuilds(repos []models.ExtendedRepository) ([]models.ExtendedRepository, error) {
	maxWorkers := config.GlobalMaxWorkers

	type job struct {
		index int
		repo  models.ExtendedRepository
	}

	jobs := make(chan job, len(repos))
	errCh := make(chan error, len(repos))

	var wg sync.WaitGroup

	worker := func() {
		defer wg.Done()
		for j := range jobs {
			resp, httpResp, err := c.api.BuildsAndDeploymentsAPI.
				GetPageOfRequiredBuildsMergeChecks(c.authCtx, j.repo.ProjectKey, j.repo.RepositorySlug).
				Execute()
			if err != nil {
				if httpResp != nil {
					c.logger.Debug("HTTP response", "status", httpResp.StatusCode, "body", httpResp.Body)
				}
				errCh <- fmt.Errorf("failed to get required builds for %s/%s: %w", j.repo.ProjectKey, j.repo.RepositorySlug, err)
				continue
			}

			if resp == nil || resp.Values == nil {
				j.repo.RequiredBuilds = nil
			} else {
				j.repo.RequiredBuilds = &resp.Values
			}

			// Update the repos slice with the updated repo (with RequiredBuilds)
			repos[j.index] = j.repo
		}
	}

	wg.Add(maxWorkers)
	for range maxWorkers {
		go worker()
	}

	for i, r := range repos {
		jobs <- job{index: i, repo: r}
	}
	close(jobs)

	wg.Wait()
	close(errCh)

	var errs []string
	for e := range errCh {
		errs = append(errs, e.Error())
	}
	if len(errs) > 0 {
		return nil, fmt.Errorf("errors occurred fetching required builds: %s", strings.Join(errs, "; "))
	}

	return repos, nil
}

// AreRequiredBuildsEqual compares two required-build conditions for equality.
// Exported for reuse across commands (e.g., diff command).
func AreRequiredBuildsEqual(a, b openapi.RestRequiredBuildCondition) bool {
	if !equalStringSlices(a.BuildParentKeys, b.BuildParentKeys) {
		return false
	}
	if !equalRefMatcher(a.RefMatcher, b.RefMatcher) {
		return false
	}
	if !equalRefMatcher(a.ExemptRefMatcher, b.ExemptRefMatcher) {
		return false
	}
	return true
}

func equalStringSlices(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func equalStringPtr(a, b *string) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	return *a == *b
}

func equalMatcherType(a, b *openapi.UpdatePullRequestCondition1RequestSourceMatcherType) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	// Assuming fields Id and Name are *string
	return equalStringPtr(a.Id, b.Id) && equalStringPtr(a.Name, b.Name)
}

func equalRefMatcher(a, b *openapi.UpdatePullRequestCondition1RequestSourceMatcher) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	if !equalStringPtr(a.Id, b.Id) {
		return false
	}
	if !equalStringPtr(a.DisplayId, b.DisplayId) {
		return false
	}
	if !equalMatcherType(a.Type, b.Type) {
		return false
	}
	return true
}
