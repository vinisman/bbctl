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

			if err != nil {
				c.logger.Debug("details", "httpResp", httpResp)
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
	for i := 0; i < maxWorkers; i++ {
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

func (c *Client) UpdateRequiredBuilds(repos []models.ExtendedRepository) error {
	maxWorkers := config.GlobalMaxWorkers

	type job struct {
		repo models.ExtendedRepository
		req  openapi.RestRequiredBuildConditionSetRequest
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
			updated, httpResp, err := c.api.BuildsAndDeploymentsAPI.
				UpdateRequiredBuildsMergeCheck(c.authCtx, j.repo.ProjectKey, j.id, j.repo.RepositorySlug).
				RestRequiredBuildConditionSetRequest(j.req).
				Execute()

			if err != nil {
				c.logger.Debug("details", "httpResp", httpResp)
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
		}
	}

	// start worker pool
	wg.Add(maxWorkers)
	for i := 0; i < maxWorkers; i++ {
		go worker()
	}

	// send all jobs to the channel
	for _, r := range repos {
		for _, wh := range *r.RequiredBuilds {
			if wh.Id != nil {
				req := toSetRequest(wh)
				jobs <- job{repo: r, req: req, id: *wh.Id}
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
		return fmt.Errorf("errors occurred updating required builds: %s", strings.Join(errs, "; "))
	}

	return nil
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
				c.logger.Debug("HTTP response",
					"status", httpResp.Status,
					"statusCode", httpResp.StatusCode,
					"body", httpResp.Body)
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
	for i := 0; i < maxWorkers; i++ {
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
				c.logger.Debug("Failed fetching required builds", "httpResp", httpResp, "error", err)
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
	for i := 0; i < maxWorkers; i++ {
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
