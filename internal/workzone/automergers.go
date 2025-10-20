package workzone

import (
	"fmt"
	"sync"

	"github.com/vinisman/bbctl/internal/config"
	"github.com/vinisman/bbctl/internal/models"
	wz "github.com/vinisman/workzone-sdk-go/client"
)

// GetRepoAutomergersList fetches branch mergerules list (settings) for a repo
func (c *Client) GetRepoAutomergersList(projectKey, repoSlug string) ([]wz.RestBranchAutoMergers, error) {
	if projectKey == "" || repoSlug == "" {
		return nil, fmt.Errorf("projectKey and repoSlug are required")
	}

	items, httpResp, err := c.api.DefaultAPI.GetBranchAutomergersList(c.ctx, projectKey, repoSlug).Execute()
	if err != nil {
		if ge, ok := err.(*wz.GenericOpenAPIError); ok {
			if config.GlobalLogger != nil {
				config.GlobalLogger.Debug("Workzone automergers raw body", "projectKey", projectKey, "repoSlug", repoSlug, "body", string(ge.Body()))
			}
		}
		if httpResp != nil {
			return nil, fmt.Errorf("get automergers list failed: http %d: %w", httpResp.StatusCode, err)
		}
		return nil, err
	}

	return items, nil
}

// GetReposAutomergers concurrently fetches mergerules for multiple repos
func (c *Client) GetReposAutomergers(repos []models.ExtendedRepository) ([]models.ExtendedRepository, error) {
	if len(repos) == 0 {
		return repos, nil
	}

	maxWorkers := config.GlobalMaxWorkers
	sem := make(chan struct{}, maxWorkers)
	var wg sync.WaitGroup
	var mu sync.Mutex
	var errs []error

	out := make([]models.ExtendedRepository, len(repos))
	copy(out, repos)

	for i := range out {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			r := out[idx]
			items, err := c.GetRepoAutomergersList(r.ProjectKey, r.RepositorySlug)
			if err != nil {
				mu.Lock()
				errs = append(errs, fmt.Errorf("%s/%s: %v", r.ProjectKey, r.RepositorySlug, err))
				mu.Unlock()
				return
			}
			mu.Lock()
			if out[idx].Workzone == nil {
				out[idx].Workzone = &models.WorkzoneData{}
			}
			out[idx].Workzone.Mergerules = items
			mu.Unlock()
		}(i)
	}

	wg.Wait()

	if len(errs) > 0 {
		return out, fmt.Errorf("errors occurred: %v", errs)
	}
	return out, nil
}

// SetBranchAutomergersList sets mergerules list for a repo
func (c *Client) SetBranchAutomergersList(projectKey, repoSlug string, items []wz.RestBranchAutoMergers) error {
	if projectKey == "" || repoSlug == "" {
		return fmt.Errorf("projectKey and repoSlug are required")
	}
	httpResp, err := c.api.DefaultAPI.SetBranchAutomergersList(c.ctx, projectKey, repoSlug).RestBranchAutoMergers(items).Execute()
	if err != nil {
		if httpResp != nil {
			return fmt.Errorf("set automergers list failed: http %d: %w", httpResp.StatusCode, err)
		}
		return err
	}
	return nil
}

// DeleteBranchAutomergersList deletes automergers list for a repo
func (c *Client) DeleteBranchAutomergersList(projectKey, repoSlug string) error {
	if projectKey == "" || repoSlug == "" {
		return fmt.Errorf("projectKey and repoSlug are required")
	}
	httpResp, err := c.api.DefaultAPI.DeleteBranchAutomergersList(c.ctx, projectKey, repoSlug).Execute()
	if err != nil {
		if httpResp != nil {
			return fmt.Errorf("delete automergers list failed: http %d: %w", httpResp.StatusCode, err)
		}
		return err
	}
	return nil
}

// SetReposAutomergers concurrently sets automergers list for multiple repos
func (c *Client) SetReposAutomergers(repos []models.ExtendedRepository) error {
	if len(repos) == 0 {
		return nil
	}
	maxWorkers := config.GlobalMaxWorkers
	sem := make(chan struct{}, maxWorkers)
	var wg sync.WaitGroup
	var mu sync.Mutex
	var errs []error

	for i := range repos {
		wg.Add(1)
		go func(r models.ExtendedRepository) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			if r.Workzone == nil || len(r.Workzone.Mergerules) == 0 {
				mu.Lock()
				errs = append(errs, fmt.Errorf("%s/%s: missing mergerules list", r.ProjectKey, r.RepositorySlug))
				mu.Unlock()
				return
			}
			if err := c.SetBranchAutomergersList(r.ProjectKey, r.RepositorySlug, r.Workzone.Mergerules); err != nil {
				mu.Lock()
				errs = append(errs, fmt.Errorf("%s/%s: %v", r.ProjectKey, r.RepositorySlug, err))
				mu.Unlock()
			}
		}(repos[i])
	}
	wg.Wait()
	if len(errs) > 0 {
		return fmt.Errorf("errors occurred: %v", errs)
	}
	return nil
}

// DeleteReposAutomergers concurrently deletes mergerules list for multiple repos
func (c *Client) DeleteReposAutomergers(repos []models.ExtendedRepository) error {
	if len(repos) == 0 {
		return nil
	}
	maxWorkers := config.GlobalMaxWorkers
	sem := make(chan struct{}, maxWorkers)
	var wg sync.WaitGroup
	var mu sync.Mutex
	var errs []error

	for i := range repos {
		wg.Add(1)
		go func(r models.ExtendedRepository) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			if err := c.DeleteBranchAutomergersList(r.ProjectKey, r.RepositorySlug); err != nil {
				mu.Lock()
				errs = append(errs, fmt.Errorf("%s/%s: %v", r.ProjectKey, r.RepositorySlug, err))
				mu.Unlock()
			}
		}(repos[i])
	}
	wg.Wait()
	if len(errs) > 0 {
		return fmt.Errorf("errors occurred: %v", errs)
	}
	return nil
}
