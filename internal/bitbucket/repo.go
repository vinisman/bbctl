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

// GetRequiredBuilds fetches required build merge checks for a given project and repository
// func (c *Client) GetRequiredBuilds(projectKey, repoSlug string) ([]openapi.RestRequiredBuildCondition, error) {
// 	if projectKey == "" || repoSlug == "" {
// 		return nil, fmt.Errorf("projectKey and repoSlug are required")
// 	}

// 	resp, _, err := c.api.BuildsAndDeploymentsAPI.
// 		GetPageOfRequiredBuildsMergeChecks(c.authCtx, projectKey, repoSlug).
// 		Execute()
// 	if err != nil {
// 		return nil, fmt.Errorf("failed to call GetPageOfRequiredBuildsMergeChecks: %w", err)
// 	}

// 	if resp == nil || resp.Values == nil {
// 		return nil, fmt.Errorf("empty response from required-builds API")
// 	}

// 	return resp.Values, nil
// }

// GetAllReposForProject fetches all repositories for a single project with pagination
// and optionally fills DefaultBranch and Webhooks for each repository
func (c *Client) GetAllReposForProject(projectKey string, options models.RepositoryOptions) ([]models.ExtendedRepository, error) {
	var (
		repos []models.ExtendedRepository
		start float32 = 0
	)

	for {
		resp, _, err := c.api.ProjectAPI.GetRepositories(c.authCtx, projectKey).
			Start(start).
			Limit(float32(c.config.PageSize)).
			Execute()
		if err != nil {
			c.logger.Error("Error fetching repositories", "project", projectKey, "error", err)
			return nil, err
		}

		if options.Repository {
			for _, r := range resp.Values {
				repos = append(repos, models.ExtendedRepository{
					RestRepository: r,
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

	// Parallel fetch DefaultBranch + Webhooks if requested
	if len(repos) > 0 {
		type enrichResult struct {
			idx int
			err error
		}
		ch := make(chan enrichResult, len(repos))

		for i, r := range repos {
			i := i
			r := r
			go func() {
				var errs []error

				// Default branch
				if options.DefaultBranch && r.RepositorySlug != "" {
					b, err := c.GetDefaultBranch(projectKey, r.RepositorySlug)
					if err == nil {
						repos[i].RestRepository.DefaultBranch = &b
					} else {
						errs = append(errs, fmt.Errorf("defaultBranch: %w", err))
					}
				}

				if options.Webhooks && r.RepositorySlug != "" {
					hooks, err := c.GetWebhooks(projectKey, r.RepositorySlug)
					if err == nil {
						repos[i].Webhooks = hooks
					} else {
						errs = append(errs, fmt.Errorf("webhooks: %w", err))
					}
				}

				// Required builds only
				if options.RequiredBuilds && r.RepositorySlug != "" {
					rb, err := c.GetRequiredBuilds(projectKey, r.RepositorySlug)
					if err == nil {
						repos[i].RequiredBuilds = rb
					} else {
						c.logger.Warn("Failed fetching required builds", "project", projectKey, "slug", r.RepositorySlug, "error", err)
					}
				}
				// Get manifest content
				if options.Manifest && r.RepositorySlug != "" && options.ManifestPath != nil {
					manifest, err := c.GetManifest(projectKey, r.RepositorySlug, *options.ManifestPath)
					if err == nil {
						repos[i].Manifest = manifest
					} else {
						c.logger.Debug("Failed fetching manifest data",
							"project", projectKey,
							"slug", r.RepositorySlug,
							"filePath", *options.ManifestPath,
							"error", err)
					}
				}
				if len(errs) > 0 {
					ch <- enrichResult{idx: i, err: fmt.Errorf("errors: %v", errs)}
				} else {
					ch <- enrichResult{idx: i}
				}
			}()
		}

		for i := 0; i < len(repos); i++ {
			res := <-ch
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

	resultsCh := make(chan result, len(slugs))

	for _, slug := range slugs {
		s := slug

		go func() {

			var r models.ExtendedRepository
			if options.Repository {
				resp, _, err := c.api.ProjectAPI.GetRepository(c.authCtx, projectKey, s).Execute()
				if err != nil {
					c.logger.Error("Error fetching repository", "project", projectKey, "slug", s, "error", err)
					resultsCh <- result{err: err}
					return
				}
				r = models.ExtendedRepository{
					RestRepository: *resp,
					ProjectKey:     projectKey,
					RepositorySlug: slug,
				}
			} else {
				r = models.ExtendedRepository{
					ProjectKey:     projectKey,
					RepositorySlug: slug,
				}
			}

			// Default branch
			if options.DefaultBranch && r.RepositorySlug != "" {
				b, err := c.GetDefaultBranch(projectKey, r.RepositorySlug)
				if err == nil {
					r.RestRepository.DefaultBranch = &b
				} else {
					c.logger.Warn("Failed fetching default branch", "project", projectKey, "slug", s, "error", err)
				}
			}

			// Webhooks only
			if options.Webhooks && r.RepositorySlug != "" {
				hooks, err := c.GetWebhooks(projectKey, r.RepositorySlug)
				if err == nil {
					r.Webhooks = hooks
				} else {
					c.logger.Warn("Failed fetching webhooks", "project", projectKey, "slug", r.RepositorySlug, "error", err)
				}
			}

			// Required builds only
			if options.RequiredBuilds && r.RepositorySlug != "" {
				rb, err := c.GetRequiredBuilds(projectKey, r.RepositorySlug)
				if err == nil {
					r.RequiredBuilds = rb
				} else {
					c.logger.Warn("Failed fetching required builds", "project", projectKey, "slug", r.RepositorySlug, "error", err)
				}
			}

			// Get manifest content
			if options.Manifest && r.RepositorySlug != "" && options.ManifestPath != nil {
				manifest, err := c.GetManifest(projectKey, r.RepositorySlug, *options.ManifestPath)
				if err == nil {
					r.Manifest = manifest
				} else {
					c.logger.Debug("Failed fetching manifest data",
						"project", projectKey,
						"slug", r.RepositorySlug,
						"filePath", *options.ManifestPath,
						"error", err)
				}
			}

			resultsCh <- result{repo: r}
		}()
	}

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
	if maxWorkers <= 0 {
		maxWorkers = 5
	}

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

				var extRepos []models.ExtendedRepository
				for _, r := range repos {
					extRepo := r

					// Default branch (уже обработан в GetAllReposForProject, но оставляем возможность дополнить)
					if options.DefaultBranch && r.RestRepository.Slug != nil && r.RestRepository.DefaultBranch == nil {
						b, err := c.GetDefaultBranch(pk, *r.RestRepository.Slug)
						if err == nil {
							extRepo.RestRepository.DefaultBranch = &b
						} else {
							c.logger.Warn("Failed fetching default branch", "project", pk, "slug", *r.RestRepository.Slug, "error", err)
						}
					}

					// Webhooks only if output != plain
					if options.Webhooks && r.RestRepository.Slug != nil {
						hooks, err := c.GetWebhooks(pk, *r.RestRepository.Slug)
						if err == nil {
							extRepo.Webhooks = hooks
						} else {
							c.logger.Warn("Failed fetching webhooks", "project", pk, "slug", *r.RestRepository.Slug, "error", err)
						}
					}

					// Required builds only
					if options.RequiredBuilds && r.RestRepository.Slug != nil {
						rb, err := c.GetRequiredBuilds(pk, *r.RestRepository.Slug)
						if err == nil {
							extRepo.RequiredBuilds = rb
						} else {
							c.logger.Warn("Failed fetching required builds", "project", pk, "slug", *r.RestRepository.Slug, "error", err)
						}
					}

					// Get manifest content
					if options.Manifest && r.RestRepository.Slug != nil && options.ManifestPath != nil {
						manifest, err := c.GetManifest(pk, *r.RestRepository.Slug, *options.ManifestPath)
						if err == nil {
							extRepo.Manifest = manifest // <-- исправлено
						} else {
							c.logger.Debug("Failed fetching manifest data",
								"project", pk,
								"slug", *r.RestRepository.Slug,
								"filePath", *options.ManifestPath,
								"error", err)
						}
					}
					extRepos = append(extRepos, extRepo)
				}

				resultsCh <- result{repos: extRepos}
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

	resp, _, err := c.api.ProjectAPI.
		GetDefaultBranch2(c.authCtx, projectKey, repoSlug).
		Execute()
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

	resp, err := http.DefaultClient.Do(req)
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
	if maxWorkers <= 0 {
		maxWorkers = 5
	}

	// Workers
	for i := 0; i < maxWorkers; i++ {
		go func() {
			for ref := range jobsCh {
				_, err := c.api.ProjectAPI.DeleteRepository(c.authCtx, ref.ProjectKey, ref.RepositorySlug).Execute()
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
func (c *Client) CreateRepos(repos []models.ExtendedRepository) error {
	type result struct {
		slug string
		name string
		err  error
	}

	resultsCh := make(chan result, len(repos))
	jobsCh := make(chan models.ExtendedRepository, len(repos))

	maxWorkers := config.GlobalMaxWorkers
	if maxWorkers <= 0 {
		maxWorkers = 5
	}

	// Workers
	for i := 0; i < maxWorkers; i++ {
		go func() {
			for r := range jobsCh {
				// Validate required fields
				if r.RestRepository.Name == nil || r.ProjectKey == "" {
					resultsCh <- result{slug: utils.SafeString(r.RestRepository.Slug), err: fmt.Errorf("name and projectKey are required")}
					continue
				}

				created, _, err := c.api.ProjectAPI.CreateRepository(c.authCtx, r.ProjectKey).
					RestRepository(r.RestRepository).
					Execute()
				if err != nil {
					resultsCh <- result{slug: utils.SafeString(r.RestRepository.Slug), name: utils.SafeString(r.RestRepository.Name), err: err}
				} else {
					resultsCh <- result{slug: utils.SafeString(created.Slug), name: utils.SafeString(r.RestRepository.Name)}
				}
			}
		}()
	}

	// Send jobs
	for _, r := range repos {
		jobsCh <- r
	}
	close(jobsCh)

	var errorsCount int
	for i := 0; i < len(repos); i++ {
		r := <-resultsCh
		if r.err != nil {
			c.logger.Error("Failed to create repository", "slug", r.slug, "name", r.name, "error", r.err)
			errorsCount++
		} else {
			c.logger.Info("Created repository", "slug", r.slug, "name", r.name)
		}
	}

	if errorsCount > 0 {
		return fmt.Errorf("failed to create %d out of %d repositories", errorsCount, len(repos))
	}
	return nil
}

// UpdateRepos updates multiple repositories in parallel by project.key + slug
func (c *Client) UpdateRepos(repos []models.ExtendedRepository) error {
	type result struct {
		slug string
		err  error
	}

	resultsCh := make(chan result, len(repos))
	jobsCh := make(chan models.ExtendedRepository, len(repos))

	maxWorkers := config.GlobalMaxWorkers
	if maxWorkers <= 0 {
		maxWorkers = 5
	}

	// Workers
	for i := 0; i < maxWorkers; i++ {
		go func() {
			for r := range jobsCh {
				// Validate required fields for update
				if r.ProjectKey == "" || r.RepositorySlug == "" {
					resultsCh <- result{slug: r.RepositorySlug, err: fmt.Errorf("project.key and slug are required for update")}
					continue
				}

				updated, httpResp, err := c.api.ProjectAPI.UpdateRepository(c.authCtx, r.ProjectKey, r.RepositorySlug).
					RestRepository(r.RestRepository).
					Execute()
				if err != nil {
					c.logger.Debug("Details", "httpResp", httpResp)
					resultsCh <- result{slug: utils.SafeString(&r.RepositorySlug), err: err}
				} else {
					resultsCh <- result{slug: utils.SafeString(updated.Slug)}
				}
			}
		}()
	}

	// Send jobs
	for _, r := range repos {
		jobsCh <- r
	}
	close(jobsCh)

	var errorsCount int
	for i := 0; i < len(repos); i++ {
		r := <-resultsCh
		if r.err != nil {
			c.logger.Error("Failed to update repository", "slug", r.slug, "error", r.err)
			errorsCount++
		} else {
			c.logger.Info("Updated repository", "slug", r.slug)
		}
	}

	if errorsCount > 0 {
		return fmt.Errorf("failed to update %d out of %d repositories", errorsCount, len(repos))
	}
	return nil
}

// ForkRepos forks multiple repositories in parallel
func (c *Client) ForkRepos(repos []models.ExtendedRepository) error {
	type result struct {
		forkName      string
		sourceProject string
		sourceSlug    string
		forkProject   string
		err           error
	}

	resultsCh := make(chan result, len(repos))
	jobsCh := make(chan models.ExtendedRepository, len(repos))

	maxWorkers := config.GlobalMaxWorkers
	if maxWorkers <= 0 {
		maxWorkers = 5
	}

	// Worker pool
	for i := 0; i < maxWorkers; i++ {
		go func() {
			for r := range jobsCh {
				if r.ProjectKey == "" || r.RepositorySlug == "" {
					resultsCh <- result{
						forkName:      utils.SafeString(r.RestRepository.Name),
						sourceProject: r.ProjectKey,
						sourceSlug:    r.RepositorySlug,
						forkProject:   "",
						err:           fmt.Errorf("sourceProject, sourceSlug are required"),
					}
					continue
				}

				createdFork, httpResp, err := c.api.ProjectAPI.ForkRepository(c.authCtx, r.ProjectKey, r.RepositorySlug).
					RestRepository(r.RestRepository).
					Execute()
				var forkName string
				if createdFork != nil && createdFork.Name != nil {
					forkName = *createdFork.Name
				} else {
					forkName = utils.SafeString(r.RestRepository.Name)
				}

				c.logger.Debug("Details", "httpResp", httpResp)
				resultsCh <- result{
					forkName:      forkName,
					sourceProject: r.ProjectKey,
					sourceSlug:    r.RepositorySlug,
					err:           err,
				}
			}
		}()
	}

	// Send jobs
	for _, r := range repos {
		jobsCh <- r
	}
	close(jobsCh)

	var errorsCount int
	for i := 0; i < len(repos); i++ {
		r := <-resultsCh
		if r.err != nil {
			c.logger.Error("Failed to fork repository",
				"sourceProject", r.sourceProject,
				"sourceSlug", r.sourceSlug,
				"forkProject", r.forkProject,
				"forkName", r.forkName,
				"error", r.err)
			errorsCount++
		} else {
			c.logger.Info("Forked repository",
				"sourceProject", r.sourceProject,
				"sourceSlug", r.sourceSlug,
				"forkProject", r.forkProject,
				"forkName", r.forkName)
		}
	}

	if errorsCount > 0 {
		return fmt.Errorf("failed to fork %d out of %d repositories", errorsCount, len(repos))
	}
	return nil
}
