package workzone

import (
	"fmt"
	"sync"

	"github.com/vinisman/bbctl/internal/config"
	"github.com/vinisman/bbctl/internal/models"
	wz "github.com/vinisman/workzone-sdk-go/client"
)

// GetRepoReviewersList fetches branch reviewers list using Workzone SDK
func (c *Client) GetRepoReviewersList(projectKey, repoSlug string) (*models.WorkzoneData, error) {
	if projectKey == "" || repoSlug == "" {
		return nil, fmt.Errorf("projectKey and repoSlug are required")
	}

	items, httpResp, err := c.api.DefaultAPI.GetBranchReviewersList(c.ctx, projectKey, repoSlug).Execute()
	if err != nil {
		// If SDK failed to unmarshal with 200 OK, log raw body for diagnostics
		if ge, ok := err.(*wz.GenericOpenAPIError); ok {
			if config.GlobalLogger != nil {
				config.GlobalLogger.Debug("Workzone reviewers raw body", "projectKey", projectKey, "repoSlug", repoSlug, "body", string(ge.Body()))
			}
		}
		if httpResp != nil {
			return nil, fmt.Errorf("get repo reviewers failed: http %d: %w", httpResp.StatusCode, err)
		}
		return nil, err
	}

	var data models.WorkzoneData
	data.Reviewers = items
	return &data, nil
}

// GetReposReviewersList concurrently fetches reviewers for multiple repos
func (c *Client) GetReposReviewersList(repos []models.ExtendedRepository) ([]models.ExtendedRepository, error) {
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
			data, err := c.GetRepoReviewersList(r.ProjectKey, r.RepositorySlug)
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
			out[idx].Workzone.Reviewers = data.Reviewers
			mu.Unlock()
		}(i)
	}

	wg.Wait()

	if len(errs) > 0 {
		return out, fmt.Errorf("errors occurred: %v", errs)
	}

	return out, nil
}

// SetBranchReviewersList sets reviewers list for a repo
func (c *Client) SetBranchReviewersList(projectKey, repoSlug string, items []wz.RestBranchReviewers) error {
	if projectKey == "" || repoSlug == "" {
		return fmt.Errorf("projectKey and repoSlug are required")
	}
	httpResp, err := c.api.DefaultAPI.SetBranchReviewersList(c.ctx, projectKey, repoSlug).RestBranchReviewers(items).Execute()
	if err != nil {
		if httpResp != nil {
			return fmt.Errorf("set reviewers list failed: http %d: %w", httpResp.StatusCode, err)
		}
		return err
	}
	return nil
}

// DeleteBranchReviewersList deletes reviewers list for a repo
func (c *Client) DeleteBranchReviewersList(projectKey, repoSlug string) error {
	if projectKey == "" || repoSlug == "" {
		return fmt.Errorf("projectKey and repoSlug are required")
	}
	httpResp, err := c.api.DefaultAPI.DeleteBranchReviewersList(c.ctx, projectKey, repoSlug).Execute()
	if err != nil {
		if httpResp != nil {
			return fmt.Errorf("delete reviewers list failed: http %d: %w", httpResp.StatusCode, err)
		}
		return err
	}
	return nil
}

// SetReposReviewersList concurrently sets reviewers list for multiple repos
func (c *Client) SetReposReviewersList(repos []models.ExtendedRepository) error {
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

			if r.Workzone == nil || len(r.Workzone.Reviewers) == 0 {
				mu.Lock()
				errs = append(errs, fmt.Errorf("%s/%s: missing reviewers list", r.ProjectKey, r.RepositorySlug))
				mu.Unlock()
				return
			}
			if err := c.SetBranchReviewersList(r.ProjectKey, r.RepositorySlug, r.Workzone.Reviewers); err != nil {
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

// DeleteReposReviewersList concurrently deletes reviewers list for multiple repos
func (c *Client) DeleteReposReviewersList(repos []models.ExtendedRepository) error {
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

			if err := c.DeleteBranchReviewersList(r.ProjectKey, r.RepositorySlug); err != nil {
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
