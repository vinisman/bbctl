package workzone

import (
	"fmt"

	"github.com/vinisman/bbctl/internal/models"
	wz "github.com/vinisman/workzone-sdk-go/client"
)

// GetRepoWorkflow fetches WorkflowProperties using Workzone SDK
// (single-repo method removed; batch method below is used by CLI)

// GetRepoWorkflows fetches WorkflowProperties for multiple repositories concurrently
func (c *Client) GetRepoWorkflows(repos []models.ExtendedRepository) ([]models.ExtendedRepository, error) {
	return batchGetOperation(
		repos,
		func(projectKey, repoSlug string) (*wz.WorkflowProperties, error) {
			props, httpResp, err := c.api.DefaultAPI.GetRepoWorkflowProperties(c.ctx, projectKey, repoSlug).Execute()
			if err != nil {
				if httpResp != nil {
					return nil, fmt.Errorf("http %d: %w", httpResp.StatusCode, err)
				}
				return nil, err
			}
			return props, nil
		},
		func(wz *models.WorkzoneData, props *wz.WorkflowProperties) {
			wz.WorkflowProperties = props
		},
	)
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
	return batchOperation(repos, func(r models.ExtendedRepository) error {
		if r.Workzone == nil || r.Workzone.WorkflowProperties == nil {
			return fmt.Errorf("%s/%s: missing workflowProperties", r.ProjectKey, r.RepositorySlug)
		}
		if err := c.SetRepoWorkflowProperties(r.ProjectKey, r.RepositorySlug, r.Workzone.WorkflowProperties); err != nil {
			return fmt.Errorf("%s/%s: %w", r.ProjectKey, r.RepositorySlug, err)
		}
		return nil
	})
}

// UpdateReposWorkflowProperties concurrently updates workflow properties for multiple repos
func (c *Client) UpdateReposWorkflowProperties(repos []models.ExtendedRepository) error {
	return batchOperation(repos, func(r models.ExtendedRepository) error {
		if r.Workzone == nil || r.Workzone.WorkflowProperties == nil {
			return fmt.Errorf("%s/%s: missing workflowProperties", r.ProjectKey, r.RepositorySlug)
		}
		if err := c.UpdateRepoWorkflowProperties(r.ProjectKey, r.RepositorySlug, r.Workzone.WorkflowProperties); err != nil {
			return fmt.Errorf("%s/%s: %w", r.ProjectKey, r.RepositorySlug, err)
		}
		return nil
	})
}

// RemoveReposWorkflowProperties concurrently removes workflow properties for multiple repos
func (c *Client) RemoveReposWorkflowProperties(repos []models.ExtendedRepository) error {
	return batchOperation(repos, func(r models.ExtendedRepository) error {
		if err := c.RemoveRepoWorkflowProperties(r.ProjectKey, r.RepositorySlug); err != nil {
			return fmt.Errorf("%s/%s: %w", r.ProjectKey, r.RepositorySlug, err)
		}
		return nil
	})
}
