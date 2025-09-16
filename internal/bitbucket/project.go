package bitbucket

import (
	"fmt"
	"sync"

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
	for range maxWorkers {
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
	for range maxWorkers {
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
func (c *Client) CreateProjects(projects []openapi.RestProject) ([]openapi.RestProject, error) {
	type result struct {
		index   int
		project *openapi.RestProject
		err     error
	}

	type job struct {
		index   int
		project openapi.RestProject
	}

	resultsCh := make(chan result, len(projects))
	jobs := make(chan job, len(projects))

	// Start workers
	maxWorkers := config.GlobalMaxWorkers
	var wg sync.WaitGroup

	for range maxWorkers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := range jobs {
				created, httpResp, err := c.api.ProjectAPI.CreateProject(c.authCtx).
					RestProject(j.project).
					Execute()
				if err != nil {
					if httpResp != nil {
						c.logger.Debug("HTTP response", "status", httpResp.StatusCode, "body", httpResp.Body)
					}
					resultsCh <- result{index: j.index, project: &j.project, err: err}
				} else {
					resultsCh <- result{index: j.index, project: created}
				}
			}
		}()
	}

	// Send jobs
	for i, p := range projects {
		jobs <- job{index: i, project: p}
	}
	close(jobs)

	// Wait for all workers to complete
	go func() {
		wg.Wait()
		close(resultsCh)
	}()

	// Collect results
	createdProjects := make([]openapi.RestProject, len(projects))
	var errorsCount int
	for r := range resultsCh {
		if r.err != nil {
			c.logger.Error("Failed to create project", "key", utils.SafeValue(r.project.Key), "error", r.err)
			errorsCount++
			// Keep original project in case of error
			if r.project != nil {
				createdProjects[r.index] = *r.project
			}
		} else {
			c.logger.Info("Created project", "key", utils.SafeValue(r.project.Key))
			createdProjects[r.index] = *r.project
		}
	}

	if errorsCount > 0 {
		return createdProjects, fmt.Errorf("failed to create %d out of %d projects", errorsCount, len(projects))
	}
	return createdProjects, nil
}

// UpdateProjects updates multiple projects in parallel
func (c *Client) UpdateProjects(projects []openapi.RestProject) ([]openapi.RestProject, error) {
	type result struct {
		index   int
		project *openapi.RestProject
		err     error
	}

	type job struct {
		index   int
		project openapi.RestProject
	}

	resultsCh := make(chan result, len(projects))
	jobs := make(chan job, len(projects))

	// Start workers
	maxWorkers := config.GlobalMaxWorkers
	var wg sync.WaitGroup

	for range maxWorkers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := range jobs {
				if j.project.Key == nil {
					resultsCh <- result{index: j.index, project: &j.project, err: fmt.Errorf("project key is required")}
					continue
				}

				updated, httpResp, err := c.api.ProjectAPI.UpdateProject(c.authCtx, *j.project.Key).
					RestProject(j.project).
					Execute()
				if err != nil && httpResp != nil {
					c.logger.Debug("HTTP response", "status", httpResp.StatusCode, "body", httpResp.Body)
				}
				if err != nil {
					resultsCh <- result{index: j.index, project: &j.project, err: err}
				} else {
					resultsCh <- result{index: j.index, project: updated}
				}
			}
		}()
	}

	// Send jobs
	for i, p := range projects {
		jobs <- job{index: i, project: p}
	}
	close(jobs)

	// Wait for all workers to complete
	go func() {
		wg.Wait()
		close(resultsCh)
	}()

	// Collect results
	updatedProjects := make([]openapi.RestProject, len(projects))
	var errorsCount int
	for r := range resultsCh {
		if r.err != nil {
			c.logger.Error("Failed to update project", "key", utils.SafeValue(r.project.Key), "error", r.err)
			errorsCount++
			// Keep original project in case of error
			if r.project != nil {
				updatedProjects[r.index] = *r.project
			}
		} else {
			c.logger.Info("Updated project", "key", utils.SafeValue(r.project.Key))
			updatedProjects[r.index] = *r.project
		}
	}

	if errorsCount > 0 {
		return updatedProjects, fmt.Errorf("failed to update %d out of %d projects", errorsCount, len(projects))
	}
	return updatedProjects, nil
}
