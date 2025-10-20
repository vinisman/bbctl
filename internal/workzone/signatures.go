package workzone

import (
	"fmt"
	"sync"

	"github.com/vinisman/bbctl/internal/config"
	"github.com/vinisman/bbctl/internal/models"
	wz "github.com/vinisman/workzone-sdk-go/client"
)

// GetRepoSignapproversList fetches branch sign approvers list (settings) for a repo
func (c *Client) GetRepoSignapproversList(projectKey, repoSlug string) ([]wz.RestBranchSignapprovers, error) {
	if projectKey == "" || repoSlug == "" {
		return nil, fmt.Errorf("projectKey and repoSlug are required")
	}

	items, httpResp, err := c.api.DefaultAPI.GetSignapproversList(c.ctx, projectKey, repoSlug).Execute()
	if err != nil {
		if ge, ok := err.(*wz.GenericOpenAPIError); ok {
			if config.GlobalLogger != nil {
				config.GlobalLogger.Debug("Workzone signapprovers raw body", "projectKey", projectKey, "repoSlug", repoSlug, "body", string(ge.Body()))
			}
		}
		if httpResp != nil {
			return nil, fmt.Errorf("get signapprovers list failed: http %d: %w", httpResp.StatusCode, err)
		}
		return nil, err
	}

	return items, nil
}

// GetReposSignatures concurrently fetches signatures for multiple repos and a PR id
func (c *Client) GetReposSignapprovers(repos []models.ExtendedRepository) ([]models.ExtendedRepository, error) {
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
			items, err := c.GetRepoSignapproversList(r.ProjectKey, r.RepositorySlug)
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
			out[idx].Workzone.Signapprovers = items
			mu.Unlock()
		}(i)
	}

	wg.Wait()

	if len(errs) > 0 {
		return out, fmt.Errorf("errors occurred: %v", errs)
	}
	return out, nil
}

// SetSignApproversList sets sign approvers list for a repo
func (c *Client) SetSignApproversList(projectKey, repoSlug string, items []wz.RestBranchSignapprovers) error {
	if projectKey == "" || repoSlug == "" {
		return fmt.Errorf("projectKey and repoSlug are required")
	}
	httpResp, err := c.api.DefaultAPI.SetSignApproversList(c.ctx, projectKey, repoSlug).RestBranchSignapprovers(items).Execute()
	if err != nil {
		if httpResp != nil {
			return fmt.Errorf("set signapprovers list failed: http %d: %w", httpResp.StatusCode, err)
		}
		return err
	}
	return nil
}

// DeleteSignapproversList deletes sign approvers list for a repo
func (c *Client) DeleteSignapproversList(projectKey, repoSlug string) error {
	if projectKey == "" || repoSlug == "" {
		return fmt.Errorf("projectKey and repoSlug are required")
	}
	httpResp, err := c.api.DefaultAPI.DeleteSignapproversList(c.ctx, projectKey, repoSlug).Execute()
	if err != nil {
		if httpResp != nil {
			return fmt.Errorf("delete signapprovers list failed: http %d: %w", httpResp.StatusCode, err)
		}
		return err
	}
	return nil
}

// SetReposSignapprovers concurrently sets sign approvers list for multiple repos
func (c *Client) SetReposSignapprovers(repos []models.ExtendedRepository) error {
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

			if r.Workzone == nil || len(r.Workzone.Signapprovers) == 0 {
				mu.Lock()
				errs = append(errs, fmt.Errorf("%s/%s: missing signapprovers list", r.ProjectKey, r.RepositorySlug))
				mu.Unlock()
				return
			}
			if err := c.SetSignApproversList(r.ProjectKey, r.RepositorySlug, r.Workzone.Signapprovers); err != nil {
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

// DeleteReposSignapprovers concurrently deletes sign approvers list for multiple repos
func (c *Client) DeleteReposSignapprovers(repos []models.ExtendedRepository) error {
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

			if err := c.DeleteSignapproversList(r.ProjectKey, r.RepositorySlug); err != nil {
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
