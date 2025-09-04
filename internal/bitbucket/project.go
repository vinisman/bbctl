package bitbucket

import (
	"fmt"

	"github.com/vinisman/bbctl/internal/config"
	"github.com/vinisman/bbctl/utils"
	openapi "github.com/vinisman/bitbucket-sdk-go/openapi"
)

// GetAllProjects fetches all projects from Bitbucket with pagination
func GetAllProjects(c *Client) ([]openapi.RestProject, error) {
	var (
		projects []openapi.RestProject
		start    float32 = 0
	)

	for {
		resp, httpResp, err := c.api.ProjectAPI.GetProjects(c.authCtx).
			Start(start).
			Limit(float32(c.config.PageSize)).
			Execute()
		if err != nil && httpResp != nil {
			c.logger.Debug("HTTP response", "status", httpResp.StatusCode, "body", httpResp.Body)
		}
		if err != nil {
			c.logger.Error("Failed to fetch projects", "error", err)
			return nil, err
		}

		projects = append(projects, resp.Values...)

		if resp.NextPageStart == nil {
			break
		}
		start = float32(*resp.NextPageStart)
	}

	return projects, nil
}

// GetProjects fetches specific projects by keys in parallel
func (c *Client) GetProjects(keys []string) ([]openapi.RestProject, error) {
	maxWorkers := config.GlobalMaxWorkers

	type job struct {
		key string
	}
	type result struct {
		project openapi.RestProject
		err     error
	}

	jobsCh := make(chan job, len(keys))
	resultsCh := make(chan result, len(keys))

	// Start workers
	for i := 0; i < maxWorkers; i++ {
		go func() {
			for j := range jobsCh {
				resp, httpResp, err := c.api.ProjectAPI.GetProject(c.authCtx, j.key).Execute()
				if err != nil && httpResp != nil {
					c.logger.Debug("HTTP response", "status", httpResp.StatusCode, "body", httpResp.Body)
				}
				if err != nil {
					c.logger.Error("Failed to fetch project", "key", j.key, "error", err)
					resultsCh <- result{err: err}
					continue
				}
				resultsCh <- result{project: *resp}
			}
		}()
	}

	// Send jobs
	for _, key := range keys {
		jobsCh <- job{key: key}
	}
	close(jobsCh)

	// Collect results
	var projects []openapi.RestProject
	var errorsCount int
	for i := 0; i < len(keys); i++ {
		r := <-resultsCh
		if r.err != nil {
			errorsCount++
			continue
		}
		projects = append(projects, r.project)
	}

	if len(projects) == 0 {
		return nil, fmt.Errorf("no projects found, errors: %d", errorsCount)
	}
	return projects, nil
}

// DeleteProjects deletes multiple projects by keys in parallel
func (c *Client) DeleteProjects(keys []string) error {
	type result struct {
		key string
		err error
	}

	maxWorkers := config.GlobalMaxWorkers

	jobsCh := make(chan string, len(keys))
	resultsCh := make(chan result, len(keys))

	// Start workers
	for i := 0; i < maxWorkers; i++ {
		go func() {
			for k := range jobsCh {
				httpResp, err := c.api.ProjectAPI.DeleteProject(c.authCtx, k).Execute()
				if err != nil && httpResp != nil {
					c.logger.Debug("HTTP response", "status", httpResp.StatusCode, "body", httpResp.Body)
				}
				if err != nil {
					resultsCh <- result{key: k, err: err}
				} else {
					resultsCh <- result{key: k}
				}
			}
		}()
	}

	// Send jobs
	for _, k := range keys {
		jobsCh <- k
	}
	close(jobsCh)

	// Collect results
	var errorsCount int
	for i := 0; i < len(keys); i++ {
		r := <-resultsCh
		if r.err != nil {
			c.logger.Error("Failed to delete project", "key", r.key, "error", r.err)
			errorsCount++
		} else {
			c.logger.Info("Deleted project", "key", r.key)
		}
	}

	if errorsCount > 0 {
		return fmt.Errorf("failed to delete %d out of %d projects", errorsCount, len(keys))
	}
	return nil
}

// CreateProjects creates multiple projects in parallel
func (c *Client) CreateProjects(projects []openapi.RestProject) error {
	type result struct {
		key string
		err error
	}

	maxWorkers := config.GlobalMaxWorkers

	jobsCh := make(chan openapi.RestProject, len(projects))
	resultsCh := make(chan result, len(projects))

	// Start workers
	for i := 0; i < maxWorkers; i++ {
		go func() {
			for p := range jobsCh {
				created, httpResp, err := c.api.ProjectAPI.CreateProject(c.authCtx).
					RestProject(p).
					Execute()
				if err != nil {
					if httpResp != nil {
						c.logger.Debug("HTTP response", "status", httpResp.StatusCode, "body", httpResp.Body)
					}
					resultsCh <- result{key: utils.SafeValue(p.Key), err: err}
				} else {
					resultsCh <- result{key: utils.SafeValue(created.Key)}
				}
			}
		}()
	}

	// Send jobs
	for _, p := range projects {
		jobsCh <- p
	}
	close(jobsCh)

	// Collect results
	var errorsCount int
	for i := 0; i < len(projects); i++ {
		r := <-resultsCh
		if r.err != nil {
			c.logger.Error("Failed to create project", "key", r.key, "error", r.err)
			errorsCount++
		} else {
			c.logger.Info("Created project", "key", r.key)
		}
	}

	if errorsCount > 0 {
		return fmt.Errorf("failed to create %d out of %d projects", errorsCount, len(projects))
	}
	return nil
}

// UpdateProjects updates multiple projects in parallel
func (c *Client) UpdateProjects(projects []openapi.RestProject) error {
	type result struct {
		key string
		err error
	}

	maxWorkers := config.GlobalMaxWorkers

	jobsCh := make(chan openapi.RestProject, len(projects))
	resultsCh := make(chan result, len(projects))

	// Start workers
	for i := 0; i < maxWorkers; i++ {
		go func() {
			for p := range jobsCh {
				if p.Key == nil {
					resultsCh <- result{key: "<nil>", err: fmt.Errorf("project key is required")}
					continue
				}

				updated, httpResp, err := c.api.ProjectAPI.UpdateProject(c.authCtx, *p.Key).
					RestProject(p).
					Execute()
				if err != nil && httpResp != nil {
					c.logger.Debug("HTTP response", "status", httpResp.StatusCode, "body", httpResp.Body)
				}
				if err != nil {
					resultsCh <- result{key: utils.SafeValue(p.Key), err: err}
				} else {
					resultsCh <- result{key: utils.SafeValue(updated.Key)}
				}
			}
		}()
	}

	// Send jobs
	for _, p := range projects {
		jobsCh <- p
	}
	close(jobsCh)

	// Collect results
	var errorsCount int
	for i := 0; i < len(projects); i++ {
		r := <-resultsCh
		if r.err != nil {
			c.logger.Error("Failed to update project", "key", r.key, "error", r.err)
			errorsCount++
		} else {
			c.logger.Info("Updated project", "key", r.key)
		}
	}

	if errorsCount > 0 {
		return fmt.Errorf("failed to update %d out of %d projects", errorsCount, len(projects))
	}
	return nil
}
