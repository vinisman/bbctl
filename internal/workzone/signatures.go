package workzone

import (
	"fmt"

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

// GetReposSignapprovers concurrently fetches signatures for multiple repos
func (c *Client) GetReposSignapprovers(repos []models.ExtendedRepository) ([]models.ExtendedRepository, error) {
	return batchGetOperation(
		repos,
		c.GetRepoSignapproversList,
		func(wz *models.WorkzoneData, items []wz.RestBranchSignapprovers) {
			wz.Signapprovers = items
		},
	)
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
	return batchOperation(repos, func(r models.ExtendedRepository) error {
		if r.Workzone == nil || len(r.Workzone.Signapprovers) == 0 {
			return fmt.Errorf("%s/%s: missing signapprovers list", r.ProjectKey, r.RepositorySlug)
		}
		if err := c.SetSignApproversList(r.ProjectKey, r.RepositorySlug, r.Workzone.Signapprovers); err != nil {
			return fmt.Errorf("%s/%s: %w", r.ProjectKey, r.RepositorySlug, err)
		}
		return nil
	})
}

// DeleteReposSignapprovers concurrently deletes sign approvers list for multiple repos
func (c *Client) DeleteReposSignapprovers(repos []models.ExtendedRepository) error {
	return batchOperation(repos, func(r models.ExtendedRepository) error {
		if err := c.DeleteSignapproversList(r.ProjectKey, r.RepositorySlug); err != nil {
			return fmt.Errorf("%s/%s: %w", r.ProjectKey, r.RepositorySlug, err)
		}
		return nil
	})
}
