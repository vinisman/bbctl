package bitbucket

import (
	"fmt"
	"net/http"
	"strings"
	"sync"

	"github.com/vinisman/bbctl/internal/config"
	"github.com/vinisman/bbctl/internal/models"
)

const defaultPageSize = 50

// workerCount returns the number of workers to use for total tasks.
func (c *Client) workerCount(total int) int {
	n := config.GlobalMaxWorkers
	if n < 1 {
		n = 1
	}
	if total > 0 && n > total {
		n = total
	}
	return n
}

// pageSize returns the configured page size, falling back to the default.
func (c *Client) pageSize() int {
	if c.config == nil || c.config.PageSize <= 0 {
		return defaultPageSize
	}
	return c.config.PageSize
}

// GetProjectPermissions fetches user and group permissions for multiple projects
// concurrently. The output order matches the input order.
func (c *Client) GetProjectPermissions(projects []models.ProjectPermissions) ([]models.PermissionRow, error) {
	if len(projects) == 0 {
		return []models.PermissionRow{}, nil
	}

	type result struct {
		index int
		rows  []models.PermissionRow
	}

	jobs := make(chan int, len(projects))
	results := make(chan result, len(projects))
	errCh := make(chan error, len(projects)*2)

	var wg sync.WaitGroup

	worker := func() {
		defer wg.Done()
		for idx := range jobs {
			p := projects[idx]
			var rows []models.PermissionRow

			users, err := c.getProjectUserPermissions(p.ProjectKey)
			if err != nil {
				errCh <- fmt.Errorf("failed to get user permissions for project %s: %w", p.ProjectKey, err)
			} else {
				rows = append(rows, users...)
			}

			groups, err := c.getProjectGroupPermissions(p.ProjectKey)
			if err != nil {
				errCh <- fmt.Errorf("failed to get group permissions for project %s: %w", p.ProjectKey, err)
			} else {
				rows = append(rows, groups...)
			}

			results <- result{index: idx, rows: rows}
		}
	}

	workers := c.workerCount(len(projects))
	wg.Add(workers)
	for range workers {
		go worker()
	}

	for i := range projects {
		jobs <- i
	}
	close(jobs)

	go func() {
		wg.Wait()
		close(results)
		close(errCh)
	}()

	ordered := make([][]models.PermissionRow, len(projects))
	for res := range results {
		ordered[res.index] = res.rows
	}

	rows := make([]models.PermissionRow, 0)
	for _, r := range ordered {
		rows = append(rows, r...)
	}

	var errs []string
	for err := range errCh {
		errs = append(errs, err.Error())
	}
	if len(errs) > 0 {
		return rows, fmt.Errorf("errors occurred fetching project permissions: %s", strings.Join(errs, "; "))
	}

	return rows, nil
}

// GetRepositoryPermissions fetches user and group permissions for multiple
// repositories concurrently. The output order matches the input order.
func (c *Client) GetRepositoryPermissions(repos []models.RepositoryPermissions) ([]models.PermissionRow, error) {
	if len(repos) == 0 {
		return []models.PermissionRow{}, nil
	}

	type result struct {
		index int
		rows  []models.PermissionRow
	}

	jobs := make(chan int, len(repos))
	results := make(chan result, len(repos))
	errCh := make(chan error, len(repos)*2)

	var wg sync.WaitGroup

	worker := func() {
		defer wg.Done()
		for idx := range jobs {
			r := repos[idx]
			var rows []models.PermissionRow

			users, err := c.getRepositoryUserPermissions(r.ProjectKey, r.RepositorySlug)
			if err != nil {
				errCh <- fmt.Errorf("failed to get user permissions for %s/%s: %w", r.ProjectKey, r.RepositorySlug, err)
			} else {
				rows = append(rows, users...)
			}

			groups, err := c.getRepositoryGroupPermissions(r.ProjectKey, r.RepositorySlug)
			if err != nil {
				errCh <- fmt.Errorf("failed to get group permissions for %s/%s: %w", r.ProjectKey, r.RepositorySlug, err)
			} else {
				rows = append(rows, groups...)
			}

			results <- result{index: idx, rows: rows}
		}
	}

	workers := c.workerCount(len(repos))
	wg.Add(workers)
	for range workers {
		go worker()
	}

	for i := range repos {
		jobs <- i
	}
	close(jobs)

	go func() {
		wg.Wait()
		close(results)
		close(errCh)
	}()

	ordered := make([][]models.PermissionRow, len(repos))
	for res := range results {
		ordered[res.index] = res.rows
	}

	rows := make([]models.PermissionRow, 0)
	for _, r := range ordered {
		rows = append(rows, r...)
	}

	var errs []string
	for err := range errCh {
		errs = append(errs, err.Error())
	}
	if len(errs) > 0 {
		return rows, fmt.Errorf("errors occurred fetching repository permissions: %s", strings.Join(errs, "; "))
	}

	return rows, nil
}

type projectPermissionJob struct {
	projectKey string
	isGroup    bool
	name       string
	permission string
}

// GrantProjectPermissions grants or updates project permissions for users and
// groups concurrently.
func (c *Client) GrantProjectPermissions(projects []models.ProjectPermissions) error {
	var jobs []projectPermissionJob
	for _, p := range projects {
		for _, u := range p.Users {
			jobs = append(jobs, projectPermissionJob{projectKey: p.ProjectKey, name: u.Name, permission: u.Permission})
		}
		for _, g := range p.Groups {
			jobs = append(jobs, projectPermissionJob{projectKey: p.ProjectKey, isGroup: true, name: g.Name, permission: g.Permission})
		}
	}

	return c.runProjectPermissionJobs(jobs, "grant")
}

// RevokeProjectPermissions revokes all project permissions for users and groups
// concurrently.
func (c *Client) RevokeProjectPermissions(projects []models.ProjectPermissions) error {
	var jobs []projectPermissionJob
	for _, p := range projects {
		for _, u := range p.Users {
			jobs = append(jobs, projectPermissionJob{projectKey: p.ProjectKey, name: u.Name})
		}
		for _, g := range p.Groups {
			jobs = append(jobs, projectPermissionJob{projectKey: p.ProjectKey, isGroup: true, name: g.Name})
		}
	}

	return c.runProjectPermissionJobs(jobs, "revoke")
}

// runProjectPermissionJobs executes project-level permission jobs in parallel.
// action is either "grant" or "revoke" and is used for logging and errors.
func (c *Client) runProjectPermissionJobs(jobs []projectPermissionJob, action string) error {
	if len(jobs) == 0 {
		return nil
	}

	jobsCh := make(chan projectPermissionJob, len(jobs))
	errCh := make(chan string, len(jobs))

	var wg sync.WaitGroup

	worker := func() {
		defer wg.Done()
		for j := range jobsCh {
			subject := "user"
			if j.isGroup {
				subject = "group"
			}

			httpResp, err := c.applyProjectPermission(j, action)
			if err != nil {
				if httpResp != nil {
					c.logHTTPResponse(httpResp)
				}
				if action == "grant" {
					errCh <- fmt.Sprintf("failed to grant %s to %s %s on project %s: %v",
						j.permission, subject, j.name, j.projectKey, err)
				} else {
					errCh <- fmt.Sprintf("failed to revoke permissions for %s %s on project %s: %v",
						subject, j.name, j.projectKey, err)
				}
				continue
			}

			if action == "grant" {
				c.logger.Info("Granted project permission",
					"project", j.projectKey, "type", subject,
					"name", j.name, "permission", j.permission)
			} else {
				c.logger.Info("Revoked project permissions",
					"project", j.projectKey, "type", subject, "name", j.name)
			}
		}
	}

	workers := c.workerCount(len(jobs))
	wg.Add(workers)
	for range workers {
		go worker()
	}

	for _, j := range jobs {
		jobsCh <- j
	}
	close(jobsCh)

	wg.Wait()
	close(errCh)

	var errs []string
	for err := range errCh {
		errs = append(errs, err)
	}
	if len(errs) > 0 {
		return fmt.Errorf("errors occurred applying project permissions: %s", strings.Join(errs, "; "))
	}

	return nil
}

func (c *Client) applyProjectPermission(j projectPermissionJob, action string) (*http.Response, error) {
	if action == "grant" {
		if j.isGroup {
			return c.api.ProjectAPI.SetPermissionForGroups1(c.authCtx, j.projectKey).
				Name(j.name).Permission(j.permission).Execute()
		}
		return c.api.ProjectAPI.SetPermissionForUsers1(c.authCtx, j.projectKey).
			Name(j.name).Permission(j.permission).Execute()
	}

	if j.isGroup {
		return c.api.ProjectAPI.RevokePermissionsForGroup1(c.authCtx, j.projectKey).Name(j.name).Execute()
	}
	return c.api.ProjectAPI.RevokePermissionsForUser1(c.authCtx, j.projectKey).Name(j.name).Execute()
}

type repositoryPermissionJob struct {
	projectKey     string
	repositorySlug string
	isGroup        bool
	names          []string
	permission     string
}

// GrantRepositoryPermissions grants or updates repository permissions for users
// and groups concurrently. Subjects with the same scope and permission level are
// sent in a single request (the repository API accepts multiple names).
func (c *Client) GrantRepositoryPermissions(repos []models.RepositoryPermissions) error {
	var jobs []repositoryPermissionJob

	for _, r := range repos {
		jobs = append(jobs, groupRepositoryPermissionJobs(r, r.Users, false)...)
		jobs = append(jobs, groupRepositoryPermissionJobs(r, r.Groups, true)...)
	}

	return c.runRepositoryPermissionJobs(jobs, "grant")
}

// groupRepositoryPermissionJobs batches subjects by permission level within one
// repository.
func groupRepositoryPermissionJobs(r models.RepositoryPermissions, entries []models.PermissionEntry, isGroup bool) []repositoryPermissionJob {
	order := make([]string, 0, len(entries))
	byPermission := map[string][]string{}

	for _, e := range entries {
		if _, ok := byPermission[e.Permission]; !ok {
			order = append(order, e.Permission)
		}
		byPermission[e.Permission] = append(byPermission[e.Permission], e.Name)
	}

	jobs := make([]repositoryPermissionJob, 0, len(order))
	for _, permission := range order {
		jobs = append(jobs, repositoryPermissionJob{
			projectKey:     r.ProjectKey,
			repositorySlug: r.RepositorySlug,
			isGroup:        isGroup,
			names:          byPermission[permission],
			permission:     permission,
		})
	}
	return jobs
}

// RevokeRepositoryPermissions revokes all repository permissions for users and
// groups concurrently.
func (c *Client) RevokeRepositoryPermissions(repos []models.RepositoryPermissions) error {
	var jobs []repositoryPermissionJob
	for _, r := range repos {
		for _, u := range r.Users {
			jobs = append(jobs, repositoryPermissionJob{
				projectKey: r.ProjectKey, repositorySlug: r.RepositorySlug, names: []string{u.Name},
			})
		}
		for _, g := range r.Groups {
			jobs = append(jobs, repositoryPermissionJob{
				projectKey: r.ProjectKey, repositorySlug: r.RepositorySlug, isGroup: true, names: []string{g.Name},
			})
		}
	}

	return c.runRepositoryPermissionJobs(jobs, "revoke")
}

// runRepositoryPermissionJobs executes repository-level permission jobs in
// parallel. action is either "grant" or "revoke".
func (c *Client) runRepositoryPermissionJobs(jobs []repositoryPermissionJob, action string) error {
	if len(jobs) == 0 {
		return nil
	}

	jobsCh := make(chan repositoryPermissionJob, len(jobs))
	errCh := make(chan string, len(jobs))

	var wg sync.WaitGroup

	worker := func() {
		defer wg.Done()
		for j := range jobsCh {
			subject := "user"
			if j.isGroup {
				subject = "group"
			}
			names := strings.Join(j.names, ",")

			httpResp, err := c.applyRepositoryPermission(j, action)
			if err != nil {
				if httpResp != nil {
					c.logHTTPResponse(httpResp)
				}
				if action == "grant" {
					errCh <- fmt.Sprintf("failed to grant %s to %s %s on %s/%s: %v",
						j.permission, subject, names, j.projectKey, j.repositorySlug, err)
				} else {
					errCh <- fmt.Sprintf("failed to revoke permissions for %s %s on %s/%s: %v",
						subject, names, j.projectKey, j.repositorySlug, err)
				}
				continue
			}

			if action == "grant" {
				c.logger.Info("Granted repository permission",
					"project", j.projectKey, "repo", j.repositorySlug,
					"type", subject, "names", names, "permission", j.permission)
			} else {
				c.logger.Info("Revoked repository permissions",
					"project", j.projectKey, "repo", j.repositorySlug,
					"type", subject, "names", names)
			}
		}
	}

	workers := c.workerCount(len(jobs))
	wg.Add(workers)
	for range workers {
		go worker()
	}

	for _, j := range jobs {
		jobsCh <- j
	}
	close(jobsCh)

	wg.Wait()
	close(errCh)

	var errs []string
	for err := range errCh {
		errs = append(errs, err)
	}
	if len(errs) > 0 {
		return fmt.Errorf("errors occurred applying repository permissions: %s", strings.Join(errs, "; "))
	}

	return nil
}

func (c *Client) applyRepositoryPermission(j repositoryPermissionJob, action string) (*http.Response, error) {
	if action == "grant" {
		if j.isGroup {
			return c.api.PermissionManagementAPI.
				SetPermissionForGroup(c.authCtx, j.projectKey, j.repositorySlug).
				Name(j.names).Permission(j.permission).Execute()
		}
		return c.api.PermissionManagementAPI.
			SetPermissionForUser(c.authCtx, j.projectKey, j.repositorySlug).
			Name(j.names).Permission(j.permission).Execute()
	}

	if j.isGroup {
		return c.api.PermissionManagementAPI.
			RevokePermissionsForGroup2(c.authCtx, j.projectKey, j.repositorySlug).
			Name(j.names[0]).Execute()
	}
	return c.api.PermissionManagementAPI.
		RevokePermissionsForUser2(c.authCtx, j.projectKey, j.repositorySlug).
		Name(j.names[0]).Execute()
}

func (c *Client) getProjectUserPermissions(projectKey string) ([]models.PermissionRow, error) {
	var rows []models.PermissionRow
	start := float32(0)
	limit := float32(c.pageSize())

	for {
		resp, httpResp, err := c.api.ProjectAPI.
			GetUsersWithAnyPermission1(c.authCtx, projectKey).
			Start(start).
			Limit(limit).
			Execute()
		if err != nil {
			if httpResp != nil {
				c.logHTTPResponse(httpResp)
			}
			return nil, err
		}

		for _, v := range resp.Values {
			name := ""
			if v.User != nil && v.User.Name != nil {
				name = *v.User.Name
			}
			permission := ""
			if v.Permission != nil {
				permission = *v.Permission
			}
			rows = append(rows, models.PermissionRow{
				ProjectKey: projectKey,
				Type:       "user",
				Name:       name,
				Permission: permission,
			})
		}

		if resp.NextPageStart == nil {
			break
		}
		start = float32(*resp.NextPageStart)
	}

	return rows, nil
}

func (c *Client) getProjectGroupPermissions(projectKey string) ([]models.PermissionRow, error) {
	var rows []models.PermissionRow
	start := float32(0)
	limit := float32(c.pageSize())

	for {
		resp, httpResp, err := c.api.ProjectAPI.
			GetGroupsWithAnyPermission1(c.authCtx, projectKey).
			Start(start).
			Limit(limit).
			Execute()
		if err != nil {
			if httpResp != nil {
				c.logHTTPResponse(httpResp)
			}
			return nil, err
		}

		for _, v := range resp.Values {
			name := ""
			if v.Group != nil && v.Group.Name != nil {
				name = *v.Group.Name
			}
			permission := ""
			if v.Permission != nil {
				permission = *v.Permission
			}
			rows = append(rows, models.PermissionRow{
				ProjectKey: projectKey,
				Type:       "group",
				Name:       name,
				Permission: permission,
			})
		}

		if resp.NextPageStart == nil {
			break
		}
		start = float32(*resp.NextPageStart)
	}

	return rows, nil
}

func (c *Client) getRepositoryUserPermissions(projectKey, repositorySlug string) ([]models.PermissionRow, error) {
	var rows []models.PermissionRow
	start := float32(0)
	limit := float32(c.pageSize())

	for {
		resp, httpResp, err := c.api.PermissionManagementAPI.
			GetUsersWithAnyPermission2(c.authCtx, projectKey, repositorySlug).
			Start(start).
			Limit(limit).
			Execute()
		if err != nil {
			if httpResp != nil {
				c.logHTTPResponse(httpResp)
			}
			return nil, err
		}

		for _, v := range resp.Values {
			name := ""
			if v.User != nil && v.User.Name != nil {
				name = *v.User.Name
			}
			permission := ""
			if v.Permission != nil {
				permission = *v.Permission
			}
			rows = append(rows, models.PermissionRow{
				ProjectKey:     projectKey,
				RepositorySlug: repositorySlug,
				Type:           "user",
				Name:           name,
				Permission:     permission,
			})
		}

		if resp.NextPageStart == nil {
			break
		}
		start = float32(*resp.NextPageStart)
	}

	return rows, nil
}

func (c *Client) getRepositoryGroupPermissions(projectKey, repositorySlug string) ([]models.PermissionRow, error) {
	var rows []models.PermissionRow
	start := float32(0)
	limit := float32(c.pageSize())

	for {
		resp, httpResp, err := c.api.PermissionManagementAPI.
			GetGroupsWithAnyPermission2(c.authCtx, projectKey, repositorySlug).
			Start(start).
			Limit(limit).
			Execute()
		if err != nil {
			if httpResp != nil {
				c.logHTTPResponse(httpResp)
			}
			return nil, err
		}

		for _, v := range resp.Values {
			name := ""
			if v.Group != nil && v.Group.Name != nil {
				name = *v.Group.Name
			}
			permission := ""
			if v.Permission != nil {
				permission = *v.Permission
			}
			rows = append(rows, models.PermissionRow{
				ProjectKey:     projectKey,
				RepositorySlug: repositorySlug,
				Type:           "group",
				Name:           name,
				Permission:     permission,
			})
		}

		if resp.NextPageStart == nil {
			break
		}
		start = float32(*resp.NextPageStart)
	}

	return rows, nil
}
