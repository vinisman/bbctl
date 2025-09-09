package bitbucket

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/vinisman/bbctl/internal/config"
	"github.com/vinisman/bbctl/internal/models"
	"github.com/vinisman/bbctl/utils"
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
		}
		if err != nil {
			c.logger.Error("Error fetching repositories", "project", projectKey, "error", err)
			return nil, err
		}

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

		for i := 0; i < len(repos); i++ {
			res := <-resultsCh
			repos[res.idx] = res.repo
			if res.err != nil {
				c.logger.Warn("Failed enriching repository", "project", projectKey, "error", res.err)
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
				var r models.ExtendedRepository
				if options.Repository {
					resp, httpResp, err := c.api.ProjectAPI.GetRepository(c.authCtx, projectKey, slug).Execute()
					if err != nil && httpResp != nil {
						c.logger.Debug("HTTP response", "status", httpResp.StatusCode, "body", httpResp.Body)
					}
					if err != nil {
						c.logger.Error("Error fetching repository", "project", projectKey, "slug", slug, "error", err)
						resultsCh <- result{err: err}
						continue
					}
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
	for i := 0; i < maxWorkers; i++ {
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

func (c *Client) GetManifest(projectKey, repoSlug, filePath string) (map[string]interface{}, error) {
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
		c.api.GetConfig().HTTPClient = &http.Client{Timeout: 15 * 1e9} // 15 seconds
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
	var parsed map[string]interface{}
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
	for i := 0; i < maxWorkers; i++ {
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

	resultsCh := make(chan result, len(repos))

	// Send jobs
	for i, r := range repos {
		go func(index int, repo models.ExtendedRepository) {
			// Validate required fields for update
			if repo.ProjectKey == "" || repo.RepositorySlug == "" {
				resultsCh <- result{index: index, repo: repo, err: fmt.Errorf("project.key and slug are required for update")}
				return
			}

			updated, httpResp, err := c.api.ProjectAPI.UpdateRepository(c.authCtx, repo.ProjectKey, repo.RepositorySlug).
				RestRepository(*repo.RestRepository).
				Execute()
			if err != nil && httpResp != nil {
				c.logger.Debug("HTTP response", "status", httpResp.StatusCode, "body", httpResp.Body)
			}
			if err != nil {
				resultsCh <- result{index: index, repo: repo, err: err}
			} else {
				// Update the repository with updated data
				repo.RestRepository = updated
				resultsCh <- result{index: index, repo: repo}
			}
		}(i, r)
	}

	// Collect results
	updatedRepos := make([]models.ExtendedRepository, len(repos))
	var errorsCount int
	for i := 0; i < len(repos); i++ {
		r := <-resultsCh
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

	resultsCh := make(chan result, len(repos))

	// Send jobs
	for i, r := range repos {
		go func(index int, repo models.ExtendedRepository) {
			if repo.ProjectKey == "" || repo.RepositorySlug == "" {
				resultsCh <- result{
					index: index,
					repo:  repo,
					err:   fmt.Errorf("sourceProject, sourceSlug are required"),
				}
				return
			}

			createdFork, httpResp, err := c.api.ProjectAPI.ForkRepository(c.authCtx, repo.ProjectKey, repo.RepositorySlug).
				RestRepository(*repo.RestRepository).
				Execute()
			if err != nil && httpResp != nil {
				c.logger.Debug("HTTP response", "status", httpResp.StatusCode, "body", httpResp.Body)
			}
			if err != nil {
				resultsCh <- result{index: index, repo: repo, err: err}
			} else {
				// Update the repository with forked data
				repo.RestRepository = createdFork
				resultsCh <- result{index: index, repo: repo}
			}
		}(i, r)
	}

	// Collect results
	forkedRepos := make([]models.ExtendedRepository, len(repos))
	var errorsCount int
	for i := 0; i < len(repos); i++ {
		r := <-resultsCh
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
	var errs []error
	// Default branch
	if options.DefaultBranch && r.RepositorySlug != "" {
		b, err := c.GetDefaultBranch(projectKey, r.RepositorySlug)
		if err == nil {
			r.RestRepository.DefaultBranch = &b
		} else {
			errs = append(errs, fmt.Errorf("defaultBranch: %w", err))
		}
	}
	// Webhooks
	if options.Webhooks && r.RepositorySlug != "" {
		updated, err := c.GetWebhooks([]models.ExtendedRepository{r})
		if err == nil && len(updated) > 0 {
			r.Webhooks = updated[0].Webhooks
		} else if err != nil {
			errs = append(errs, fmt.Errorf("webhooks: %w", err))
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
		}
	}
	if len(errs) > 0 {
		return r, fmt.Errorf("enrichment errors: %v", errs)
	}
	return r, nil
}
