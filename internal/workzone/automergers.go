package workzone

import (
	"bytes"
	"encoding/json"
	"fmt"

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
	return batchGetOperation(
		repos,
		c.GetRepoAutomergersList,
		func(wz *models.WorkzoneData, items []wz.RestBranchAutoMergers) {
			wz.Mergerules = items
		},
	)
}

// SetBranchAutomergersList sets mergerules list for a repo
func (c *Client) SetBranchAutomergersList(projectKey, repoSlug string, items []wz.RestBranchAutoMergers) error {
	if projectKey == "" || repoSlug == "" {
		return fmt.Errorf("projectKey and repoSlug are required")
	}
	httpResp, err := c.api.DefaultAPI.SetBranchAutomergersList(c.ctx, projectKey, repoSlug).RestBranchAutoMergers(items).Execute()
	if err != nil {
		if ge, ok := err.(*wz.GenericOpenAPIError); ok {
			if config.GlobalLogger != nil {
				config.GlobalLogger.Debug("Workzone automergers set error body", "projectKey", projectKey, "repoSlug", repoSlug, "body", string(ge.Body()))
			}
		}
		// Fallback: some Workzone versions respond 500 while actually applying the change.
		if httpResp != nil && httpResp.StatusCode == 500 {
			current, _, getErr := c.api.DefaultAPI.GetBranchAutomergersList(c.ctx, projectKey, repoSlug).Execute()
			if getErr == nil {
				wantJSON, _ := json.Marshal(items)
				gotJSON, _ := json.Marshal(current)
				if bytes.Equal(wantJSON, gotJSON) {
					if config.GlobalLogger != nil {
						config.GlobalLogger.Warn("Workzone returned HTTP 500 but state matches desired mergerules; treating as success", "projectKey", projectKey, "repoSlug", repoSlug)
					}
					return nil
				}
			}
		}
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
	return batchOperation(repos, func(r models.ExtendedRepository) error {
		if r.Workzone == nil || len(r.Workzone.Mergerules) == 0 {
			return fmt.Errorf("%s/%s: missing mergerules list", r.ProjectKey, r.RepositorySlug)
		}
		if err := c.SetBranchAutomergersList(r.ProjectKey, r.RepositorySlug, r.Workzone.Mergerules); err != nil {
			return fmt.Errorf("%s/%s: %w", r.ProjectKey, r.RepositorySlug, err)
		}
		return nil
	})
}

// DeleteReposAutomergers concurrently deletes mergerules list for multiple repos
func (c *Client) DeleteReposAutomergers(repos []models.ExtendedRepository) error {
	return batchOperation(repos, func(r models.ExtendedRepository) error {
		if err := c.DeleteBranchAutomergersList(r.ProjectKey, r.RepositorySlug); err != nil {
			return fmt.Errorf("%s/%s: %w", r.ProjectKey, r.RepositorySlug, err)
		}
		return nil
	})
}
