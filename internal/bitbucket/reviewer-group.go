package bitbucket

import (
	"fmt"
	"strings"
	"sync"

	"github.com/vinisman/bbctl/internal/config"
	"github.com/vinisman/bbctl/internal/models"
	openapi "github.com/vinisman/bitbucket-sdk-go/openapi"
)

// GetReviewerGroups fetches all reviewer groups for given repositories
func (c *Client) GetReviewerGroups(repos []models.ExtendedRepository) ([]models.ExtendedRepository, error) {
	var errs []string

	for i := range repos {
		resp, httpResp, err := c.api.PullRequestsAPI.GetReviewerGroups1(
			c.authCtx,
			repos[i].ProjectKey,
			repos[i].RepositorySlug,
		).Execute()

		if err != nil {
			if httpResp != nil {
				c.logger.Debug("HTTP response", "status", httpResp.StatusCode, "body", httpResp.Body)
			}
			errs = append(errs, fmt.Sprintf("failed to get reviewer groups for %s/%s: %v",
				repos[i].ProjectKey, repos[i].RepositorySlug, err))
			continue
		}

		reviewerGroups := resp.Values
		repos[i].ReviewerGroups = &reviewerGroups
		c.logger.Info("Retrieved reviewer groups",
			"project", repos[i].ProjectKey,
			"repo", repos[i].RepositorySlug,
			"count", len(reviewerGroups))
	}

	if len(errs) > 0 {
		return repos, fmt.Errorf("errors occurred fetching reviewer groups: %s", strings.Join(errs, "; "))
	}

	return repos, nil
}

// CreateReviewerGroups creates new reviewer groups concurrently for multiple repositories
func (c *Client) CreateReviewerGroups(repos []models.ExtendedRepository) ([]models.ExtendedRepository, error) {
	maxWorkers := config.GlobalMaxWorkers

	// Enrich user data with IDs (required by API)
	// Only Name and Id fields are required by Bitbucket API for reviewer groups
	for i := range repos {
		if repos[i].ReviewerGroups == nil {
			continue
		}
		for j := range *repos[i].ReviewerGroups {
			rg := &(*repos[i].ReviewerGroups)[j]
			if len(rg.Users) == 0 {
				continue
			}
			// Fetch user IDs for users that don't have ID set
			var enrichedUsers []openapi.ApplicationUser
			for _, user := range rg.Users {
				// If user already has ID, use as-is
				if user.Id != nil {
					enrichedUsers = append(enrichedUsers, user)
					continue
				}

				if user.Name == nil || *user.Name == "" {
					continue
				}

				// Get user ID from Bitbucket
				userResp, httpResp, err := c.api.PermissionManagementAPI.GetUsers1(c.authCtx).Filter(*user.Name).Execute()
				if err != nil {
					c.logger.Warn("Failed to get user ID",
						"username", *user.Name,
						"error", err)
					if httpResp != nil {
						c.logger.Debug("HTTP response", "status", httpResp.StatusCode)
					}
					continue
				}
				if userResp != nil && len(userResp.Values) > 0 {
					// Only set Name and Id - the minimum required fields
					appUser := openapi.ApplicationUser{
						Name: user.Name,
						Id:   userResp.Values[0].Id,
					}
					enrichedUsers = append(enrichedUsers, appUser)
					c.logger.Debug("Enriched user with ID",
						"username", *user.Name,
						"id", userResp.Values[0].Id)
				}
			}
			rg.Users = enrichedUsers
		}
	}

	type job struct {
		repoIndex     int
		repo          models.ExtendedRepository
		reviewerGroup openapi.RestReviewerGroup
	}

	// count the total number of reviewer group tasks
	var total int
	for _, r := range repos {
		if r.ReviewerGroups != nil {
			total += len(*r.ReviewerGroups)
		}
	}

	jobs := make(chan job, total)
	errCh := make(chan error, total)

	var mu sync.Mutex
	var wg sync.WaitGroup

	newRepos := make([]models.ExtendedRepository, len(repos))
	for i := range repos {
		newRepos[i].ProjectKey = repos[i].ProjectKey
		newRepos[i].RepositorySlug = repos[i].RepositorySlug
		newRepos[i].ReviewerGroups = &[]openapi.RestReviewerGroup{}
	}

	worker := func() {
		defer wg.Done()
		for j := range jobs {
			c.logger.Debug("Creating reviewer group",
				"project", j.repo.ProjectKey,
				"repo", j.repo.RepositorySlug,
				"name", *j.reviewerGroup.Name,
				"users_count", len(j.reviewerGroup.Users))

			created, httpResp, err := c.api.PullRequestsAPI.
				Create2(c.authCtx, j.repo.ProjectKey, j.repo.RepositorySlug).
				RestReviewerGroup(j.reviewerGroup).
				Execute()

			if err != nil {
				c.logger.Error("failed to create reviewer group",
					"error", err,
					"project", j.repo.ProjectKey,
					"repo", j.repo.RepositorySlug,
					"name", *j.reviewerGroup.Name,
					"users", j.reviewerGroup.Users)
				if httpResp != nil {
					c.logger.Debug("HTTP response", "status", httpResp.StatusCode, "body", httpResp.Body)
				}
				errCh <- fmt.Errorf("failed to create reviewer group %s in %s/%s: %w",
					*j.reviewerGroup.Name, j.repo.ProjectKey, j.repo.RepositorySlug, err)
				continue
			}

			mu.Lock()
			*newRepos[j.repoIndex].ReviewerGroups = append(*newRepos[j.repoIndex].ReviewerGroups, *created)
			mu.Unlock()

			c.logger.Info("Created reviewer group",
				"project", j.repo.ProjectKey,
				"repo", j.repo.RepositorySlug,
				"name", *created.Name,
				"id", *created.Id)
		}
	}

	// Start workers
	for range maxWorkers {
		wg.Add(1)
		go worker()
	}

	// Send jobs
	for i, repo := range repos {
		if repo.ReviewerGroups == nil {
			continue
		}
		for _, rg := range *repo.ReviewerGroups {
			jobs <- job{
				repoIndex:     i,
				repo:          repo,
				reviewerGroup: rg,
			}
		}
	}
	close(jobs)

	// Wait for all workers to complete
	wg.Wait()
	close(errCh)

	// Collect errors
	var errs []error
	for err := range errCh {
		errs = append(errs, err)
	}

	if len(errs) > 0 {
		return newRepos, fmt.Errorf("errors occurred creating reviewer groups: %v", errs)
	}

	c.logger.Info("Successfully created all reviewer groups")
	return newRepos, nil
}

// UpdateReviewerGroups updates existing reviewer groups concurrently for multiple repositories
func (c *Client) UpdateReviewerGroups(repos []models.ExtendedRepository) ([]models.ExtendedRepository, error) {
	maxWorkers := config.GlobalMaxWorkers

	// Enrich user data with IDs (required by API)
	// Only Name and Id fields are required by Bitbucket API for reviewer groups
	for i := range repos {
		if repos[i].ReviewerGroups == nil {
			continue
		}
		for j := range *repos[i].ReviewerGroups {
			rg := &(*repos[i].ReviewerGroups)[j]
			if len(rg.Users) == 0 {
				continue
			}
			// Fetch user IDs for users that don't have ID set
			var enrichedUsers []openapi.ApplicationUser
			for _, user := range rg.Users {
				// If user already has ID, use as-is
				if user.Id != nil {
					enrichedUsers = append(enrichedUsers, user)
					continue
				}

				if user.Name == nil || *user.Name == "" {
					continue
				}

				// Get user ID from Bitbucket
				userResp, httpResp, err := c.api.PermissionManagementAPI.GetUsers1(c.authCtx).Filter(*user.Name).Execute()
				if err != nil {
					c.logger.Warn("Failed to get user ID",
						"username", *user.Name,
						"error", err)
					if httpResp != nil {
						c.logger.Debug("HTTP response", "status", httpResp.StatusCode)
					}
					continue
				}
				if userResp != nil && len(userResp.Values) > 0 {
					// Only set Name and Id - the minimum required fields
					appUser := openapi.ApplicationUser{
						Name: user.Name,
						Id:   userResp.Values[0].Id,
					}
					enrichedUsers = append(enrichedUsers, appUser)
					c.logger.Debug("Enriched user with ID",
						"username", *user.Name,
						"id", userResp.Values[0].Id)
				}
			}
			rg.Users = enrichedUsers
		}
	}

	type job struct {
		repoIndex     int
		repo          models.ExtendedRepository
		reviewerGroup openapi.RestReviewerGroup
	}

	// count the total number of reviewer group tasks
	var total int
	for _, r := range repos {
		if r.ReviewerGroups != nil {
			total += len(*r.ReviewerGroups)
		}
	}

	jobs := make(chan job, total)
	errCh := make(chan error, total)

	var mu sync.Mutex
	var wg sync.WaitGroup

	newRepos := make([]models.ExtendedRepository, len(repos))
	for i := range repos {
		newRepos[i].ProjectKey = repos[i].ProjectKey
		newRepos[i].RepositorySlug = repos[i].RepositorySlug
		newRepos[i].ReviewerGroups = &[]openapi.RestReviewerGroup{}
	}

	worker := func() {
		defer wg.Done()
		for j := range jobs {
			if j.reviewerGroup.Id == nil {
				groupName := "unknown"
				if j.reviewerGroup.Name != nil {
					groupName = *j.reviewerGroup.Name
				}
				errCh <- fmt.Errorf("reviewer group %s in %s/%s is missing ID",
					groupName, j.repo.ProjectKey, j.repo.RepositorySlug)
				continue
			}

			updated, httpResp, err := c.api.PullRequestsAPI.
				Update2(c.authCtx, j.repo.ProjectKey, fmt.Sprintf("%d", *j.reviewerGroup.Id), j.repo.RepositorySlug).
				RestReviewerGroup(j.reviewerGroup).
				Execute()

			groupName := "unknown"
			if j.reviewerGroup.Name != nil {
				groupName = *j.reviewerGroup.Name
			}

			if err != nil {
				c.logger.Error("failed to update reviewer group",
					"error", err,
					"project", j.repo.ProjectKey,
					"repo", j.repo.RepositorySlug,
					"name", groupName,
					"id", *j.reviewerGroup.Id)
				if httpResp != nil {
					c.logger.Debug("HTTP response", "status", httpResp.StatusCode, "body", httpResp.Body)
				}
				errCh <- fmt.Errorf("failed to update reviewer group %s (ID: %d) in %s/%s: %w",
					groupName, *j.reviewerGroup.Id, j.repo.ProjectKey, j.repo.RepositorySlug, err)
				continue
			}

			mu.Lock()
			*newRepos[j.repoIndex].ReviewerGroups = append(*newRepos[j.repoIndex].ReviewerGroups, *updated)
			mu.Unlock()

			updatedName := "unknown"
			if updated.Name != nil {
				updatedName = *updated.Name
			}

			c.logger.Info("Updated reviewer group",
				"project", j.repo.ProjectKey,
				"repo", j.repo.RepositorySlug,
				"name", updatedName,
				"id", *updated.Id)
		}
	}

	// Start workers
	for range maxWorkers {
		wg.Add(1)
		go worker()
	}

	// Send jobs
	for i, repo := range repos {
		if repo.ReviewerGroups == nil {
			continue
		}
		for _, rg := range *repo.ReviewerGroups {
			jobs <- job{
				repoIndex:     i,
				repo:          repo,
				reviewerGroup: rg,
			}
		}
	}
	close(jobs)

	// Wait for all workers to complete
	wg.Wait()
	close(errCh)

	// Collect errors
	var errs []error
	for err := range errCh {
		errs = append(errs, err)
	}

	if len(errs) > 0 {
		return newRepos, fmt.Errorf("errors occurred updating reviewer groups: %v", errs)
	}

	c.logger.Info("Successfully updated all reviewer groups")
	return newRepos, nil
}

// DeleteReviewerGroups deletes reviewer groups concurrently for multiple repositories
func (c *Client) DeleteReviewerGroups(repos []models.ExtendedRepository) ([]models.ExtendedRepository, error) {
	maxWorkers := config.GlobalMaxWorkers

	type job struct {
		repoIndex     int
		repo          models.ExtendedRepository
		reviewerGroup openapi.RestReviewerGroup
	}

	// count the total number of reviewer group tasks
	var total int
	for _, r := range repos {
		if r.ReviewerGroups != nil {
			total += len(*r.ReviewerGroups)
		}
	}

	jobs := make(chan job, total)
	errCh := make(chan error, total)

	var mu sync.Mutex
	var wg sync.WaitGroup

	newRepos := make([]models.ExtendedRepository, len(repos))
	for i := range repos {
		newRepos[i].ProjectKey = repos[i].ProjectKey
		newRepos[i].RepositorySlug = repos[i].RepositorySlug
		newRepos[i].ReviewerGroups = &[]openapi.RestReviewerGroup{}
	}

	worker := func() {
		defer wg.Done()
		for j := range jobs {
			if j.reviewerGroup.Id == nil {
				groupName := "unknown"
				if j.reviewerGroup.Name != nil {
					groupName = *j.reviewerGroup.Name
				}
				errCh <- fmt.Errorf("reviewer group %s in %s/%s is missing ID",
					groupName, j.repo.ProjectKey, j.repo.RepositorySlug)
				continue
			}

			httpResp, err := c.api.PullRequestsAPI.
				Delete7(c.authCtx, j.repo.ProjectKey, fmt.Sprintf("%d", *j.reviewerGroup.Id), j.repo.RepositorySlug).
				Execute()

			groupName := "unknown"
			if j.reviewerGroup.Name != nil {
				groupName = *j.reviewerGroup.Name
			}

			if err != nil {
				c.logger.Error("failed to delete reviewer group",
					"error", err,
					"project", j.repo.ProjectKey,
					"repo", j.repo.RepositorySlug,
					"name", groupName,
					"id", *j.reviewerGroup.Id)
				if httpResp != nil {
					c.logger.Debug("HTTP response", "status", httpResp.StatusCode, "body", httpResp.Body)
				}
				errCh <- fmt.Errorf("failed to delete reviewer group %s (ID: %d) in %s/%s: %w",
					groupName, *j.reviewerGroup.Id, j.repo.ProjectKey, j.repo.RepositorySlug, err)
				continue
			}

			mu.Lock()
			*newRepos[j.repoIndex].ReviewerGroups = append(*newRepos[j.repoIndex].ReviewerGroups, j.reviewerGroup)
			mu.Unlock()

			c.logger.Info("Deleted reviewer group",
				"project", j.repo.ProjectKey,
				"repo", j.repo.RepositorySlug,
				"name", groupName,
				"id", *j.reviewerGroup.Id)
		}
	}

	// Start workers
	for range maxWorkers {
		wg.Add(1)
		go worker()
	}

	// Send jobs
	for i, repo := range repos {
		if repo.ReviewerGroups == nil {
			continue
		}
		for _, rg := range *repo.ReviewerGroups {
			jobs <- job{
				repoIndex:     i,
				repo:          repo,
				reviewerGroup: rg,
			}
		}
	}
	close(jobs)

	// Wait for all workers to complete
	wg.Wait()
	close(errCh)

	// Collect errors
	var errs []error
	for err := range errCh {
		errs = append(errs, err)
	}

	if len(errs) > 0 {
		return newRepos, fmt.Errorf("errors occurred deleting reviewer groups: %v", errs)
	}

	c.logger.Info("Successfully deleted all reviewer groups")
	return newRepos, nil
}
