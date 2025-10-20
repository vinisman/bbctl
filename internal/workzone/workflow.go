package workzone

import (
	"fmt"
	"sync"

	"github.com/vinisman/bbctl/internal/config"
	"github.com/vinisman/bbctl/internal/models"
	wz "github.com/vinisman/workzone-sdk-go/client"
)

// GetRepoWorkflow fetches WorkflowProperties using Workzone SDK
// (single-repo method removed; batch method below is used by CLI)

// GetRepoWorkflows fetches WorkflowProperties for multiple repositories concurrently
func (c *Client) GetRepoWorkflows(repos []models.ExtendedRepository) ([]models.ExtendedRepository, error) {
	if len(repos) == 0 {
		return repos, nil
	}

	maxWorkers := config.GlobalMaxWorkers

	sem := make(chan struct{}, maxWorkers)
	var mu sync.Mutex
	var errs []error

	out := make([]models.ExtendedRepository, len(repos))
	copy(out, repos)

	var wg sync.WaitGroup
	for i := range out {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			r := out[idx]
			props, httpResp, err := c.api.DefaultAPI.GetRepoWorkflowProperties(c.ctx, r.ProjectKey, r.RepositorySlug).Execute()
			if err != nil {
				mu.Lock()
				if httpResp != nil {
					errs = append(errs, fmt.Errorf("%s/%s: http %d: %w", r.ProjectKey, r.RepositorySlug, httpResp.StatusCode, err))
				} else {
					errs = append(errs, fmt.Errorf("%s/%s: %w", r.ProjectKey, r.RepositorySlug, err))
				}
				mu.Unlock()
				return
			}

			mu.Lock()
			out[idx].Workzone = &models.WorkzoneData{WorkflowProperties: props}
			mu.Unlock()
		}(i)
	}

	wg.Wait()

	if len(errs) > 0 {
		return out, fmt.Errorf("errors occurred: %v", errs)
	}

	return out, nil
}

// SetRepoWorkflowProperties sets workflow properties for a repo
func (c *Client) SetRepoWorkflowProperties(projectKey, repoSlug string, props *wz.WorkflowProperties) error {
	if projectKey == "" || repoSlug == "" || props == nil {
		return fmt.Errorf("projectKey, repoSlug and props are required")
	}
	httpResp, err := c.api.DefaultAPI.SetRepoWorkflowProperties(c.ctx, projectKey, repoSlug).WorkflowProperties(*props).Execute()
	if err != nil {
		if httpResp != nil {
			return fmt.Errorf("set workflow properties failed: http %d: %w", httpResp.StatusCode, err)
		}
		return err
	}
	return nil
}

// UpdateRepoWorkflowProperties updates workflow properties for a repo
func (c *Client) UpdateRepoWorkflowProperties(projectKey, repoSlug string, props *wz.WorkflowProperties) error {
	if projectKey == "" || repoSlug == "" || props == nil {
		return fmt.Errorf("projectKey, repoSlug and props are required")
	}
	httpResp, err := c.api.DefaultAPI.UpdateRepoWorkflowProperties(c.ctx, projectKey, repoSlug).WorkflowProperties(*props).Execute()
	if err != nil {
		if httpResp != nil {
			return fmt.Errorf("update workflow properties failed: http %d: %w", httpResp.StatusCode, err)
		}
		return err
	}
	return nil
}

// RemoveRepoWorkflowProperties deletes workflow properties for a repo
func (c *Client) RemoveRepoWorkflowProperties(projectKey, repoSlug string) error {
	if projectKey == "" || repoSlug == "" {
		return fmt.Errorf("projectKey and repoSlug are required")
	}
	httpResp, err := c.api.DefaultAPI.RemoveRepoWorkflowProperties(c.ctx, projectKey, repoSlug).Execute()
	if err != nil {
		if httpResp != nil {
			return fmt.Errorf("remove workflow properties failed: http %d: %w", httpResp.StatusCode, err)
		}
		return err
	}
	return nil
}

// SetReposWorkflowProperties concurrently sets workflow properties for multiple repos
func (c *Client) SetReposWorkflowProperties(repos []models.ExtendedRepository) error {
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

			if r.Workzone == nil || r.Workzone.WorkflowProperties == nil {
				mu.Lock()
				errs = append(errs, fmt.Errorf("%s/%s: missing workflowProperties", r.ProjectKey, r.RepositorySlug))
				mu.Unlock()
				return
			}
			if err := c.SetRepoWorkflowProperties(r.ProjectKey, r.RepositorySlug, r.Workzone.WorkflowProperties); err != nil {
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

// UpdateReposWorkflowProperties concurrently updates workflow properties for multiple repos
func (c *Client) UpdateReposWorkflowProperties(repos []models.ExtendedRepository) error {
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

			if r.Workzone == nil || r.Workzone.WorkflowProperties == nil {
				mu.Lock()
				errs = append(errs, fmt.Errorf("%s/%s: missing workflowProperties", r.ProjectKey, r.RepositorySlug))
				mu.Unlock()
				return
			}
			if err := c.UpdateRepoWorkflowProperties(r.ProjectKey, r.RepositorySlug, r.Workzone.WorkflowProperties); err != nil {
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

// RemoveReposWorkflowProperties concurrently removes workflow properties for multiple repos
func (c *Client) RemoveReposWorkflowProperties(repos []models.ExtendedRepository) error {
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

			if err := c.RemoveRepoWorkflowProperties(r.ProjectKey, r.RepositorySlug); err != nil {
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
