package bitbucket

import (
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/vinisman/bbctl/internal/config"
	"github.com/vinisman/bbctl/internal/models"
	"github.com/vinisman/bbctl/utils"
	openapi "github.com/vinisman/bitbucket-sdk-go/openapi"
	"gopkg.in/yaml.v3"
)

// GetAllReposForProject fetches all repositories for a single project with pagination
// and optionally fills DefaultBranch and Webhooks for each repository
func (c *Client) GetAllReposForProject(projectKey string, options models.RepositoryOptions) ([]models.ExtendedRepository, error) {
	var (
		repos []models.ExtendedRepository
		start float32 = 0
	)

	for {
		resp, httpResp, err := c.api.ProjectAPI.GetRepositories(c.authCtx, projectKey).
			Start(start).
			Limit(float32(c.config.PageSize)).
			Execute()
		if err != nil && httpResp != nil {
			c.logger.Debug("HTTP response", "status", httpResp.StatusCode, "body", httpResp.Body)
			httpResp.Body.Close()
		}
		if err != nil {
			c.logger.Error("Error fetching repositories", "project", projectKey, "error", err)
			return nil, err
		}
		httpResp.Body.Close()

		if options.Repository {
			for _, r := range resp.Values {
				repos = append(repos, models.ExtendedRepository{
					RestRepository: &r,
					RepositorySlug: *r.Slug,
					ProjectKey:     projectKey,
				})
			}
		} else {
			for _, r := range resp.Values {
				repos = append(repos, models.ExtendedRepository{
					RepositorySlug: *r.Slug,
					ProjectKey:     projectKey,
				})
			}
		}

		if resp.NextPageStart == nil {
			break
		}
		start = float32(*resp.NextPageStart)
	}

	// Parallel enrichment if requested, using a worker pool
	if len(repos) > 0 {
		type enrichResult struct {
			idx  int
			repo models.ExtendedRepository
			err  error
		}
		maxWorkers := config.GlobalMaxWorkers

		jobsCh := make(chan int, len(repos))
		resultsCh := make(chan enrichResult, len(repos))

		for w := 0; w < maxWorkers; w++ {
			go func() {
				for i := range jobsCh {
					r, err := c.enrichRepository(repos[i], projectKey, options)
					resultsCh <- enrichResult{idx: i, repo: r, err: err}
				}
			}()
		}

		for i := range repos {
			jobsCh <- i
		}
		close(jobsCh)

		// Collect results and return on first error
		for i := 0; i < len(repos); i++ {
			res := <-resultsCh
			repos[res.idx] = res.repo
			if res.err != nil {
				// Drain remaining results to avoid goroutine leaks
				for j := i + 1; j < len(repos); j++ {
					<-resultsCh
				}
				return nil, res.err
			}
		}
	}

	return repos, nil
}

// GetReposBySlugs fetches specific repositories by project key and slugs in parallel
func (c *Client) GetReposBySlugs(projectKey string, slugs []string, options models.RepositoryOptions) ([]models.ExtendedRepository, error) {
	type result struct {
		repo models.ExtendedRepository
		err  error
	}

	maxWorkers := config.GlobalMaxWorkers

	jobsCh := make(chan string, len(slugs))
	resultsCh := make(chan result, len(slugs))

	// Worker pool
	for w := 0; w < maxWorkers; w++ {
		go func() {
			for slug := range jobsCh {
				// Always fetch repository to verify it exists and belongs to the correct project.
				resp, httpResp, err := c.api.ProjectAPI.GetRepository(c.authCtx, projectKey, slug).Execute()
				if err != nil && httpResp != nil {
					c.logger.Debug("HTTP response", "status", httpResp.StatusCode, "body", httpResp.Body)
				}
				if err != nil {
					c.logger.Error("Error fetching repository", "project", projectKey, "slug", slug, "error", err)
					resultsCh <- result{err: err}
					continue
				}
				if resp.Project != nil && !strings.EqualFold(resp.Project.Key, projectKey) {
					c.logger.Error("Repository not found in requested project (may have been moved)",
						"requested_project", projectKey,
						"actual_project", resp.Project.Key,
						"slug", slug)
					resultsCh <- result{err: fmt.Errorf("repository %s not found in project %s (found in %s, it may have been moved)", slug, projectKey, resp.Project.Key)}
					continue
				}
				var r models.ExtendedRepository
				if options.Repository {
					r = models.ExtendedRepository{
						RestRepository: resp,
						ProjectKey:     projectKey,
						RepositorySlug: slug,
					}
				} else {
					r = models.ExtendedRepository{
						ProjectKey:     projectKey,
						RepositorySlug: slug,
					}
				}

				enriched, err := c.enrichRepository(r, projectKey, options)
				resultsCh <- result{repo: enriched, err: err}
			}
		}()
	}

	// Send all slugs as jobs
	for _, slug := range slugs {
		jobsCh <- slug
	}
	close(jobsCh)

	var repos []models.ExtendedRepository
	var errorsCount int

	for i := 0; i < len(slugs); i++ {
		r := <-resultsCh
		if r.err != nil {
			errorsCount++
			continue
		}
		repos = append(repos, r.repo)
	}

	if len(repos) == 0 {
		return nil, fmt.Errorf("no repositories found for project %s, errors: %d", projectKey, errorsCount)
	}

	return repos, nil
}

// GetAllRepos fetches all repositories for multiple projects in parallel with worker pool
func (c *Client) GetAllRepos(projectKeys []string, options models.RepositoryOptions) ([]models.ExtendedRepository, error) {
	type result struct {
		repos []models.ExtendedRepository
		err   error
	}

	resultsCh := make(chan result, len(projectKeys))
	jobsCh := make(chan string, len(projectKeys))

	maxWorkers := config.GlobalMaxWorkers

	// Worker pool
	for range maxWorkers {
		go func() {
			for pk := range jobsCh {
				repos, err := c.GetAllReposForProject(pk, options)
				if err != nil {
					c.logger.Error("Failed fetching repositories for project", "project", pk, "error", err)
					resultsCh <- result{err: err}
					continue
				}
				// All enrichment already performed in GetAllReposForProject, just append.
				resultsCh <- result{repos: repos}
			}
		}()
	}

	// Send jobs
	for _, pk := range projectKeys {
		jobsCh <- strings.TrimSpace(pk)
	}
	close(jobsCh)

	var allRepos []models.ExtendedRepository
	var errorsCount int

	// Collect results
	for i := 0; i < len(projectKeys); i++ {
		r := <-resultsCh
		if r.err != nil {
			errorsCount++
			continue
		}
		allRepos = append(allRepos, r.repos...)
	}

	if len(allRepos) == 0 {
		return nil, fmt.Errorf("no repositories found, errors: %d", errorsCount)
	}

	return allRepos, nil
}

// GetDefaultBranch fetches the default branch for a repository
func (c *Client) GetDefaultBranch(projectKey, repoSlug string) (string, error) {
	if projectKey == "" || repoSlug == "" {
		return "", fmt.Errorf("projectKey and repoSlug must be provided")
	}

	resp, httpResp, err := c.api.ProjectAPI.
		GetDefaultBranch2(c.authCtx, projectKey, repoSlug).
		Execute()
	if err != nil && httpResp != nil {
		c.logger.Debug("HTTP response", "status", httpResp.StatusCode, "body", httpResp.Body)
	}
	if err != nil {
		c.logger.Error("Failed to get default branch",
			"projectKey", projectKey,
			"repoSlug", repoSlug,
			"error", err)
		return "", err
	}

	if resp != nil && resp.DisplayId != nil {
		return *resp.DisplayId, nil
	}
	return "", nil
}

func (c *Client) GetManifest(projectKey, repoSlug, filePath string) (map[string]any, error) {
	if projectKey == "" || repoSlug == "" || filePath == "" {
		return nil, fmt.Errorf("projectKey, repoSlug and filePath must be provided")
	}

	baseURL := c.api.GetConfig().Servers[0].URL
	url := fmt.Sprintf("%s/api/1.0/projects/%s/repos/%s/raw/%s",
		strings.TrimRight(baseURL, "/"), projectKey, repoSlug, filePath)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}

	// Apply headers from SDK config
	for k, v := range c.api.GetConfig().DefaultHeader {
		req.Header.Set(k, v)
	}

	// Ensure httpClient is set and has a reasonable timeout
	if c.api.GetConfig().HTTPClient == nil {
		timeout := time.Duration(15 * 1e9) // 15 seconds
		httpClient := &http.Client{Timeout: timeout}
		if config.GlobalCfg.Insecure {
			transport := &http.Transport{
				TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
			}
			httpClient.Transport = transport
		}
		c.api.GetConfig().HTTPClient = httpClient
	}

	resp, err := c.api.GetConfig().HTTPClient.Do(req)
	if err != nil {
		c.logger.Error("Failed to fetch file",
			"projectKey", projectKey,
			"repoSlug", repoSlug,
			"filePath", filePath,
			"error", err)
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to fetch file: HTTP %d", resp.StatusCode)
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	// Parse manifest based on file extension
	var parsed map[string]any
	switch {
	case strings.HasSuffix(strings.ToLower(filePath), ".json"):
		if err := json.Unmarshal(data, &parsed); err != nil {
			return nil, fmt.Errorf("failed to parse JSON manifest: %w", err)
		}
	case strings.HasSuffix(strings.ToLower(filePath), ".yaml"),
		strings.HasSuffix(strings.ToLower(filePath), ".yml"):
		if err := yaml.Unmarshal(data, &parsed); err != nil {
			return nil, fmt.Errorf("failed to parse YAML manifest: %w", err)
		}
	default:
		return nil, fmt.Errorf("unsupported manifest format (expected .json/.yaml/.yml)")
	}
	return parsed, nil
}

// DeleteRepos deletes multiple repositories by project + slug in parallel
func (c *Client) DeleteRepos(refs []models.ExtendedRepository) error {
	type result struct {
		project string
		slug    string
		err     error
	}

	resultsCh := make(chan result, len(refs))
	jobsCh := make(chan models.ExtendedRepository, len(refs))

	maxWorkers := config.GlobalMaxWorkers

	// Workers
	for range maxWorkers {
		go func() {
			for ref := range jobsCh {
				httpResp, err := c.api.ProjectAPI.DeleteRepository(c.authCtx, ref.ProjectKey, ref.RepositorySlug).Execute()
				if err != nil && httpResp != nil {
					c.logger.Debug("HTTP response", "status", httpResp.StatusCode, "body", httpResp.Body)
				}
				if err != nil {
					resultsCh <- result{project: ref.ProjectKey, slug: ref.RepositorySlug, err: err}
				} else {
					resultsCh <- result{project: ref.ProjectKey, slug: ref.RepositorySlug}
				}
			}
		}()
	}

	// Send jobs
	for _, ref := range refs {
		jobsCh <- ref
	}
	close(jobsCh)

	var errorsCount int
	for i := 0; i < len(refs); i++ {
		r := <-resultsCh
		if r.err != nil {
			c.logger.Error("Failed to delete repository",
				"project", r.project,
				"slug", r.slug,
				"error", r.err)
			errorsCount++
		} else {
			c.logger.Info("Deleted repository",
				"project", r.project,
				"slug", r.slug)
		}
	}

	if errorsCount > 0 {
		return fmt.Errorf("failed to delete %d out of %d repositories", errorsCount, len(refs))
	}

	return nil
}

// CreateRepos creates multiple repositories in parallel
func (c *Client) CreateRepos(repos []models.ExtendedRepository) ([]models.ExtendedRepository, error) {
	type result struct {
		index int
		repo  models.ExtendedRepository
		err   error
	}

	resultsCh := make(chan result, len(repos))

	// Send jobs
	for i, r := range repos {
		go func(index int, repo models.ExtendedRepository) {
			// Validate required fields
			if repo.RestRepository.Name == nil || repo.ProjectKey == "" {
				resultsCh <- result{index: index, repo: repo, err: fmt.Errorf("name and projectKey are required")}
				return
			}

			created, httpResp, err := c.api.ProjectAPI.CreateRepository(c.authCtx, repo.ProjectKey).
				RestRepository(*repo.RestRepository).
				Execute()
			if err != nil && httpResp != nil {
				c.logger.Debug("HTTP response", "status", httpResp.StatusCode, "body", httpResp.Body)
			}
			if err != nil {
				resultsCh <- result{index: index, repo: repo, err: err}
			} else {
				// Update the repository with created data
				repo.RestRepository = created
				resultsCh <- result{index: index, repo: repo}
			}
		}(i, r)
	}

	// Collect results
	createdRepos := make([]models.ExtendedRepository, len(repos))
	var errorsCount int
	for i := 0; i < len(repos); i++ {
		r := <-resultsCh
		if r.err != nil {
			c.logger.Error("Failed to create repository", "slug", utils.SafeValue(r.repo.RestRepository.Slug), "name", utils.SafeValue(r.repo.RestRepository.Name), "error", r.err)
			errorsCount++
			// Keep original repository in case of error
			createdRepos[r.index] = r.repo
		} else {
			c.logger.Info("Created repository", "slug", utils.SafeValue(r.repo.RestRepository.Slug), "name", utils.SafeValue(r.repo.RestRepository.Name))
			createdRepos[r.index] = r.repo
		}
	}

	if errorsCount > 0 {
		return createdRepos, fmt.Errorf("failed to create %d out of %d repositories", errorsCount, len(repos))
	}
	return createdRepos, nil
}

// UpdateRepos updates multiple repositories in parallel by project.key + slug
func (c *Client) UpdateRepos(repos []models.ExtendedRepository) ([]models.ExtendedRepository, error) {
	type result struct {
		index int
		repo  models.ExtendedRepository
		err   error
	}

	type job struct {
		index int
		repo  models.ExtendedRepository
	}

	resultsCh := make(chan result, len(repos))
	jobs := make(chan job, len(repos))

	// Start workers
	maxWorkers := config.GlobalMaxWorkers
	var wg sync.WaitGroup

	for range maxWorkers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := range jobs {
				// Validate required fields for update
				if j.repo.ProjectKey == "" || j.repo.RepositorySlug == "" {
					resultsCh <- result{index: j.index, repo: j.repo, err: fmt.Errorf("project.key and slug are required for update")}
					continue
				}

				updated, httpResp, err := c.api.ProjectAPI.UpdateRepository(c.authCtx, j.repo.ProjectKey, j.repo.RepositorySlug).
					RestRepository(*j.repo.RestRepository).
					Execute()
				if err != nil && httpResp != nil {
					c.logger.Debug("HTTP response", "status", httpResp.StatusCode, "body", httpResp.Body)
				}
				if err != nil {
					resultsCh <- result{index: j.index, repo: j.repo, err: err}
				} else {
					// Update the repository with updated data
					j.repo.RestRepository = updated
					resultsCh <- result{index: j.index, repo: j.repo}
				}
			}
		}()
	}

	// Send jobs
	for i, r := range repos {
		jobs <- job{index: i, repo: r}
	}
	close(jobs)

	// Wait for workers to complete in main thread, then close results channel
	wg.Wait()
	close(resultsCh)

	// Collect results
	updatedRepos := make([]models.ExtendedRepository, len(repos))
	var errorsCount int
	for r := range resultsCh {
		if r.err != nil {
			c.logger.Error("Failed to update repository", "slug", r.repo.RepositorySlug, "error", r.err)
			errorsCount++
			// Keep original repository in case of error
			updatedRepos[r.index] = r.repo
		} else {
			c.logger.Info("Updated repository", "slug", r.repo.RepositorySlug)
			updatedRepos[r.index] = r.repo
		}
	}

	if errorsCount > 0 {
		return updatedRepos, fmt.Errorf("failed to update %d out of %d repositories", errorsCount, len(repos))
	}
	return updatedRepos, nil
}

// ForkRepos forks multiple repositories in parallel
func (c *Client) ForkRepos(repos []models.ExtendedRepository) ([]models.ExtendedRepository, error) {
	type result struct {
		index int
		repo  models.ExtendedRepository
		err   error
	}

	type job struct {
		index int
		repo  models.ExtendedRepository
	}

	resultsCh := make(chan result, len(repos))
	jobs := make(chan job, len(repos))

	// Start workers
	maxWorkers := config.GlobalMaxWorkers
	var wg sync.WaitGroup

	for range maxWorkers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := range jobs {
				if j.repo.ProjectKey == "" || j.repo.RepositorySlug == "" {
					resultsCh <- result{
						index: j.index,
						repo:  j.repo,
						err:   fmt.Errorf("sourceProject, sourceSlug are required"),
					}
					continue
				}

				createdFork, httpResp, err := c.api.ProjectAPI.ForkRepository(c.authCtx, j.repo.ProjectKey, j.repo.RepositorySlug).
					RestRepository(*j.repo.RestRepository).
					Execute()
				if err != nil && httpResp != nil {
					c.logger.Debug("HTTP response", "status", httpResp.StatusCode, "body", httpResp.Body)
				}
				if err != nil {
					resultsCh <- result{index: j.index, repo: j.repo, err: err}
				} else {
					// Update the repository with forked data
					j.repo.RestRepository = createdFork
					resultsCh <- result{index: j.index, repo: j.repo}
				}
			}
		}()
	}

	// Send jobs
	for i, r := range repos {
		jobs <- job{index: i, repo: r}
	}
	close(jobs)

	// Wait for workers to complete in main thread, then close results channel
	wg.Wait()
	close(resultsCh)

	// Collect results
	forkedRepos := make([]models.ExtendedRepository, len(repos))
	var errorsCount int
	for r := range resultsCh {
		if r.err != nil {
			c.logger.Error("Failed to fork repository",
				"sourceProject", r.repo.ProjectKey,
				"sourceSlug", r.repo.RepositorySlug,
				"forkName", utils.SafeValue(r.repo.RestRepository.Name),
				"error", r.err)
			errorsCount++
			// Keep original repository in case of error
			forkedRepos[r.index] = r.repo
		} else {
			c.logger.Info("Forked repository",
				"sourceProject", r.repo.ProjectKey,
				"sourceSlug", r.repo.RepositorySlug,
				"forkName", utils.SafeValue(r.repo.RestRepository.Name))
			forkedRepos[r.index] = r.repo
		}
	}

	if errorsCount > 0 {
		return forkedRepos, fmt.Errorf("failed to fork %d out of %d repositories", errorsCount, len(repos))
	}
	return forkedRepos, nil
}

// enrichRepository enriches the given ExtendedRepository with additional data according to options.
func (c *Client) enrichRepository(r models.ExtendedRepository, projectKey string, options models.RepositoryOptions) (models.ExtendedRepository, error) {
	// Default branch
	if options.DefaultBranch && r.RepositorySlug != "" {
		b, err := c.GetDefaultBranch(projectKey, r.RepositorySlug)
		if err == nil {
			// write both flat field and nested restRepository.defaultBranch if repository requested
			r.DefaultBranch = b
			if r.RestRepository != nil {
				r.RestRepository.DefaultBranch = &b
			}
		} else {
			return r, fmt.Errorf("defaultBranch: %w", err)
		}
	}
	// Webhooks
	if options.Webhooks && r.RepositorySlug != "" {
		updated, err := c.GetWebhooks([]models.ExtendedRepository{r})
		if err == nil && len(updated) > 0 {
			r.Webhooks = updated[0].Webhooks
		} else if err != nil {
			return r, fmt.Errorf("webhooks: %w", err)
		}
	}
	// Required builds only
	if options.RequiredBuilds && r.RepositorySlug != "" {
		rbList, err := c.GetRequiredBuilds([]models.ExtendedRepository{r})
		if err == nil && len(rbList) > 0 {
			r.RequiredBuilds = rbList[0].RequiredBuilds
		} else if err != nil {
			c.logger.Warn("Failed fetching required builds", "project", projectKey, "slug", r.RepositorySlug, "error", err)
		}
	}
	// Get manifest content
	if options.Manifest && r.RepositorySlug != "" && options.ManifestPath != nil {
		manifest, err := c.GetManifest(projectKey, r.RepositorySlug, *options.ManifestPath)
		if err == nil {
			r.Manifest = &manifest
		} else {
			c.logger.Debug("Failed fetching manifest data",
				"project", projectKey,
				"slug", r.RepositorySlug,
				"filePath", *options.ManifestPath,
				"error", err)
			return r, fmt.Errorf("manifest: %w", err)
		}
	}

	// Get config files content as separate output sections
	if options.ConfigFiles && r.RepositorySlug != "" && len(options.ConfigFileMap) > 0 {
		configs := make(map[string]any, len(options.ConfigFileMap))
		for key, configPath := range options.ConfigFileMap {
			cfg, err := c.GetManifest(projectKey, r.RepositorySlug, configPath)
			if err == nil {
				configs[key] = cfg
			} else {
				c.logger.Debug("Failed fetching config file data",
					"project", projectKey,
					"slug", r.RepositorySlug,
					"filePath", configPath,
					"error", err)
				return r, fmt.Errorf("config %s: %w", key, err)
			}
		}
		if len(configs) > 0 {
			r.ConfigFiles = &configs
		}
	}

	return r, nil
}

// GetBranchPermissions fetches all branch permissions for multiple repositories
func (c *Client) GetBranchPermissions(repos []models.ExtendedRepository) ([]models.ExtendedRepository, error) {
	var errs []string

	for i := range repos {
		resp, httpResp, err := c.api.RepositoryAPI.
			GetRestrictions1(c.authCtx, repos[i].ProjectKey, repos[i].RepositorySlug).
			Execute()
		if err != nil && httpResp != nil {
			c.logger.Debug("HTTP response", "status", httpResp.StatusCode, "body", httpResp.Body)
		}
		if err != nil {
			errs = append(errs, fmt.Sprintf("failed to get branch permissions for %s/%s: %v", repos[i].ProjectKey, repos[i].RepositorySlug, err))
			continue
		}

		repos[i].BranchPermissions = &resp.Values
	}

	if len(errs) > 0 {
		return nil, fmt.Errorf("errors occurred fetching branch permissions: %s", strings.Join(errs, "; "))
	}

	return repos, nil
}

// CreateBranchPermissions creates new branch permissions concurrently for multiple repositories
func (c *Client) CreateBranchPermissions(repos []models.ExtendedRepository) ([]models.ExtendedRepository, error) {
	maxWorkers := config.GlobalMaxWorkers

	type job struct {
		repoIndex  int
		repo       models.ExtendedRepository
		permission openapi.RestRefRestriction
	}

	type result struct {
		repoIndex  int
		permission openapi.RestRefRestriction
	}

	// count the total number of permission tasks
	var total int
	for _, r := range repos {
		if r.BranchPermissions != nil {
			total += len(*r.BranchPermissions)
		}
	}

	if total == 0 {
		return []models.ExtendedRepository{}, nil
	}

	jobs := make(chan job, total)
	resultsCh := make(chan result, total)
	errCh := make(chan error, total)

	var wg sync.WaitGroup

	worker := func() {
		defer wg.Done()
		for j := range jobs {
			// Convert RestRefRestriction to RestRefRestrictionCreate
			restriction := openapi.RestRefRestrictionCreate{
				Type:    j.permission.Type,
				Matcher: j.permission.Matcher,
				Scope:   j.permission.Scope,
				Groups:  j.permission.Groups,
			}
			// Convert users from objects to strings (usernames)
			for _, u := range j.permission.Users {
				if u.Name != nil {
					restriction.Users = append(restriction.Users, *u.Name)
				}
			}

			created, httpResp, err := c.api.RepositoryAPI.
				CreateRestrictions1WithUserNames(c.authCtx, j.repo.ProjectKey, j.repo.RepositorySlug, []openapi.RestRefRestrictionCreate{restriction})

			if err != nil {
				c.logger.Error("failed to create branch permission",
					"error", err,
					"project", j.repo.ProjectKey,
					"repo", j.repo.RepositorySlug,
					"type", utils.SafeValue(j.permission.Type))
				if httpResp != nil {
					c.logger.Debug("HTTP response", "status", httpResp.StatusCode, "body", httpResp.Body)
				}
				errCh <- err
				continue
			}

			// created is []RestRefRestriction, take first one
			if len(created) > 0 {
				resultsCh <- result{repoIndex: j.repoIndex, permission: created[0]}

				c.logger.Info("Created branch permission",
					"project", j.repo.ProjectKey,
					"repo", j.repo.RepositorySlug,
					"id", utils.SafeValue(created[0].Id),
					"type", utils.SafeValue(created[0].Type))
			}
		}
	}

	// start worker pool
	wg.Add(maxWorkers)
	for range maxWorkers {
		go worker()
	}

	// send all jobs to the channel
	for i, r := range repos {
		if r.BranchPermissions != nil {
			for _, perm := range *r.BranchPermissions {
				jobs <- job{repoIndex: i, repo: r, permission: perm}
			}
		}
	}
	close(jobs)

	// wait for workers and close channels
	wg.Wait()
	close(resultsCh)
	close(errCh)

	// collect results into newRepos
	newRepos := make([]models.ExtendedRepository, len(repos))
	for i := range repos {
		newRepos[i].ProjectKey = repos[i].ProjectKey
		newRepos[i].RepositorySlug = repos[i].RepositorySlug
		newRepos[i].BranchPermissions = &[]openapi.RestRefRestriction{}
	}

	for res := range resultsCh {
		*newRepos[res.repoIndex].BranchPermissions = append(*newRepos[res.repoIndex].BranchPermissions, res.permission)
	}

	// collect all errors
	var firstErr error
	for e := range errCh {
		if firstErr == nil {
			firstErr = e
		}
	}

	// filter newRepos: only those with at least one permission
	createdRepos := []models.ExtendedRepository{}
	for _, r := range newRepos {
		if len(*r.BranchPermissions) > 0 {
			createdRepos = append(createdRepos, r)
		}
	}
	return createdRepos, firstErr
}

// UpdateBranchPermissions updates existing branch permissions concurrently
// Uses the same CreateRestrictions1WithUserNames API as create (Bitbucket upsert behavior)
func (c *Client) UpdateBranchPermissions(repos []models.ExtendedRepository) ([]models.ExtendedRepository, error) {
	maxWorkers := config.GlobalMaxWorkers

	type job struct {
		repoIndex  int
		repo       models.ExtendedRepository
		permission openapi.RestRefRestriction
	}

	type result struct {
		repoIndex  int
		permission openapi.RestRefRestriction
	}

	// count total tasks
	var total int
	for _, r := range repos {
		if r.BranchPermissions != nil {
			total += len(*r.BranchPermissions)
		}
	}

	if total == 0 {
		return []models.ExtendedRepository{}, nil
	}

	jobs := make(chan job, total)
	resultsCh := make(chan result, total)
	errCh := make(chan error, total)

	var wg sync.WaitGroup

	worker := func() {
		defer wg.Done()
		for j := range jobs {
			if j.permission.Id == nil {
				errCh <- fmt.Errorf("permission ID is required for update in %s/%s", j.repo.ProjectKey, j.repo.RepositorySlug)
				continue
			}

			// Convert RestRefRestriction to RestRefRestrictionCreate
			restriction := openapi.RestRefRestrictionCreate{
				Id:      j.permission.Id,
				Type:    j.permission.Type,
				Matcher: j.permission.Matcher,
				Scope:   j.permission.Scope,
				Groups:  j.permission.Groups,
			}
			// Convert users from objects to strings (usernames)
			for _, u := range j.permission.Users {
				if u.Name != nil {
					restriction.Users = append(restriction.Users, *u.Name)
				}
			}

			updated, httpResp, err := c.api.RepositoryAPI.
				CreateRestrictions1WithUserNames(c.authCtx, j.repo.ProjectKey, j.repo.RepositorySlug, []openapi.RestRefRestrictionCreate{restriction})

			if err != nil {
				if httpResp != nil {
					c.logger.Debug("HTTP response", "status", httpResp.StatusCode, "body", httpResp.Body)
					if httpResp.StatusCode == 404 {
						c.logger.Info("Permission not found during update, skipping",
							"project", j.repo.ProjectKey,
							"repo", j.repo.RepositorySlug,
							"id", utils.Int32PtrToString(j.permission.Id))
						continue
					}
				}
				errCh <- fmt.Errorf("failed to update branch permission %s in %s/%s: %w", utils.Int32PtrToString(j.permission.Id), j.repo.ProjectKey, j.repo.RepositorySlug, err)
				continue
			}

			// updated is []RestRefRestriction, take first one
			if len(updated) > 0 {
				resultsCh <- result{repoIndex: j.repoIndex, permission: updated[0]}

				c.logger.Info("Updated branch permission",
					"project", j.repo.ProjectKey,
					"repo", j.repo.RepositorySlug,
					"id", utils.SafeValue(updated[0].Id),
					"type", utils.SafeValue(updated[0].Type))
			}
		}
	}

	// start worker pool
	wg.Add(maxWorkers)
	for range maxWorkers {
		go worker()
	}

	// send all jobs to the channel
	for i, r := range repos {
		if r.BranchPermissions != nil {
			for _, perm := range *r.BranchPermissions {
				jobs <- job{repoIndex: i, repo: r, permission: perm}
			}
		}
	}
	close(jobs)

	// wait for workers and close channels
	wg.Wait()
	close(resultsCh)
	close(errCh)

	// collect results into newRepos
	newRepos := make([]models.ExtendedRepository, len(repos))
	for i := range repos {
		newRepos[i].ProjectKey = repos[i].ProjectKey
		newRepos[i].RepositorySlug = repos[i].RepositorySlug
		newRepos[i].BranchPermissions = &[]openapi.RestRefRestriction{}
	}

	for res := range resultsCh {
		*newRepos[res.repoIndex].BranchPermissions = append(*newRepos[res.repoIndex].BranchPermissions, res.permission)
	}

	// collect first error
	var firstErr error
	for e := range errCh {
		if firstErr == nil {
			firstErr = e
		}
	}

	// filter newRepos: only those with at least one permission
	updatedRepos := []models.ExtendedRepository{}
	for _, r := range newRepos {
		if len(*r.BranchPermissions) > 0 {
			updatedRepos = append(updatedRepos, r)
		}
	}

	return updatedRepos, firstErr
}

// DeleteBranchPermissions deletes branch permissions concurrently by ID
func (c *Client) DeleteBranchPermissions(repos []models.ExtendedRepository) error {
	maxWorkers := config.GlobalMaxWorkers

	type job struct {
		repo       models.ExtendedRepository
		permission openapi.RestRefRestriction
	}

	// count total tasks
	var total int
	for _, r := range repos {
		if r.BranchPermissions != nil {
			total += len(*r.BranchPermissions)
		}
	}

	jobs := make(chan job, total)
	errCh := make(chan error, total)

	var wg sync.WaitGroup

	worker := func() {
		defer wg.Done()
		for j := range jobs {
			if j.permission.Id == nil {
				c.logger.Error("Delete branch permission failed",
					"project", j.repo.ProjectKey,
					"repo", j.repo.RepositorySlug,
					"id", nil,
					"error", "permission ID is required")
				errCh <- fmt.Errorf("delete permission failed: missing id")
				continue
			}

			httpResp, err := c.api.RepositoryAPI.
				DeleteRestriction1(c.authCtx, j.repo.ProjectKey, utils.Int32PtrToString(j.permission.Id), j.repo.RepositorySlug).
				Execute()
			if err != nil {
				if httpResp != nil {
					c.logger.Debug("HTTP response", "status", httpResp.StatusCode, "body", httpResp.Body)
					if httpResp.StatusCode == 404 {
						c.logger.Info("Branch permission was already absent (404), skipping",
							"project", j.repo.ProjectKey,
							"repo", j.repo.RepositorySlug,
							"id", utils.Int32PtrToString(j.permission.Id))
						continue
					}
				}
				c.logger.Error("Delete branch permission failed",
					"project", j.repo.ProjectKey,
					"repo", j.repo.RepositorySlug,
					"id", utils.Int32PtrToString(j.permission.Id),
					"error", err)
				errCh <- err
				continue
			}

			c.logger.Info("Deleted branch permission (or was not present)",
				"project", j.repo.ProjectKey,
				"repo", j.repo.RepositorySlug,
				"id", utils.Int32PtrToString(j.permission.Id))
		}
	}

	// start worker pool
	wg.Add(maxWorkers)
	for range maxWorkers {
		go worker()
	}

	// send all jobs to the channel
	for _, r := range repos {
		if r.BranchPermissions != nil {
			for _, perm := range *r.BranchPermissions {
				jobs <- job{repo: r, permission: perm}
			}
		}
	}
	close(jobs)

	wg.Wait()
	close(errCh)

	// collect all errors
	var errs []string
	for e := range errCh {
		errs = append(errs, e.Error())
	}
	if len(errs) > 0 {
		return fmt.Errorf("errors occurred deleting branch permissions: %s", strings.Join(errs, "; "))
	}
	return nil
}
