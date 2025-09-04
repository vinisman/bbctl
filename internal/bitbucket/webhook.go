package bitbucket

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"

	"github.com/vinisman/bbctl/internal/config"
	"github.com/vinisman/bbctl/internal/models"
	"github.com/vinisman/bbctl/utils"
	openapi "github.com/vinisman/bitbucket-sdk-go/openapi"
)

// Find webhooks fetches all webhooks for a given project and repository

func (c *Client) GetWebhooks(repos []models.ExtendedRepository) ([]models.ExtendedRepository, error) {
	var errs []string

	for i := range repos {
		httpResp, err := c.api.RepositoryAPI.FindWebhooks1(c.authCtx, repos[i].ProjectKey, repos[i].RepositorySlug).Execute()
		if err != nil {
			errs = append(errs, fmt.Sprintf("failed to call FindWebhooks1 for %s/%s: %v", repos[i].ProjectKey, repos[i].RepositorySlug, err))
			continue
		}
		defer httpResp.Body.Close()

		if httpResp.StatusCode != http.StatusOK {
			bodyBytes, _ := io.ReadAll(httpResp.Body)
			errs = append(errs, fmt.Sprintf("unexpected status %d for %s/%s: %s", httpResp.StatusCode, repos[i].ProjectKey, repos[i].RepositorySlug, string(bodyBytes)))
			continue
		}

		bodyBytes, err := io.ReadAll(httpResp.Body)
		if err != nil {
			errs = append(errs, fmt.Sprintf("failed to read response body for %s/%s: %v", repos[i].ProjectKey, repos[i].RepositorySlug, err))
			continue
		}

		c.logger.Debug("Details", "bodyBytes", bodyBytes)

		var webhooksResp models.WebhookResponse
		if err := json.Unmarshal(bodyBytes, &webhooksResp); err != nil {
			errs = append(errs, fmt.Sprintf("failed to parse webhook response JSON for %s/%s: %v", repos[i].ProjectKey, repos[i].RepositorySlug, err))
			continue
		}

		repos[i].Webhooks = &webhooksResp.Values
	}

	if len(errs) > 0 {
		return nil, fmt.Errorf("errors occurred fetching webhooks: %s", strings.Join(errs, "; "))
	}

	return repos, nil
}

// CreateWebhook creates new webhooks concurrently for multiple repositories
func (c *Client) CreateWebhooks(repos []models.ExtendedRepository) ([]models.ExtendedRepository, error) {
	maxWorkers := config.GlobalMaxWorkers

	type job struct {
		repoIndex int
		repo      models.ExtendedRepository
		webhook   openapi.RestWebhook
	}

	// count the total number of webhook tasks
	var total int
	for _, r := range repos {
		total += len(*r.Webhooks)
	}

	jobs := make(chan job, total)
	errCh := make(chan error, total)

	var mu sync.Mutex
	var wg sync.WaitGroup

	newRepos := make([]models.ExtendedRepository, len(repos))
	for i := range repos {
		newRepos[i].ProjectKey = repos[i].ProjectKey
		newRepos[i].RepositorySlug = repos[i].RepositorySlug
		newRepos[i].Webhooks = &[]openapi.RestWebhook{}
	}

	worker := func() {
		defer wg.Done()
		for j := range jobs {
			created, httpResp, err := c.api.RepositoryAPI.
				CreateWebhook1(c.authCtx, j.repo.ProjectKey, j.repo.RepositorySlug).
				RestWebhook(j.webhook).
				Execute()

			if err != nil {
				c.logger.Error("failed to create webhook",
					"error", err,
					"project", j.repo.ProjectKey,
					"repo", j.repo.RepositorySlug,
					"name", utils.SafeValue(j.webhook.Name))
				c.logger.Debug("HTTP response details",
					"status", httpResp.Status,
					"statusCode", httpResp.StatusCode,
					"body", httpResp.Body)
				errCh <- err
				continue
			}

			mu.Lock()
			*newRepos[j.repoIndex].Webhooks = append(*newRepos[j.repoIndex].Webhooks, *created)
			mu.Unlock()

			c.logger.Info("Created webhook",
				"project", j.repo.ProjectKey,
				"repo", j.repo.RepositorySlug,
				"id", utils.SafeValue(created.Id),
				"name", utils.SafeValue(created.Name))
		}
	}

	// start worker pool
	wg.Add(maxWorkers)
	for i := 0; i < maxWorkers; i++ {
		go worker()
	}

	// send all jobs to the channel
	for i, r := range repos {
		for _, wh := range *r.Webhooks {
			jobs <- job{repoIndex: i, repo: r, webhook: wh}
		}
	}
	close(jobs)

	wg.Wait()
	close(errCh)

	// collect all errors
	var firstErr error
	for e := range errCh {
		if firstErr == nil {
			firstErr = e
		}
	}

	// filter newRepos: only those with at least one webhook
	createdRepos := []models.ExtendedRepository{}
	for _, r := range newRepos {
		if len(*r.Webhooks) > 0 {
			createdRepos = append(createdRepos, r)
		}
	}
	return createdRepos, firstErr
}

// UpdateWebhook updates existing webhooks concurrently by updating all webhooks listed in repos.Webhooks
func (c *Client) UpdateWebhooks(repos []models.ExtendedRepository) error {
	maxWorkers := config.GlobalMaxWorkers

	type job struct {
		repo    models.ExtendedRepository
		webhook openapi.RestWebhook
	}

	// count total tasks
	var total int
	for _, r := range repos {
		total += len(*r.Webhooks)
	}

	jobs := make(chan job, total)
	errCh := make(chan error, total)

	var wg sync.WaitGroup

	worker := func() {
		defer wg.Done()
		for j := range jobs {
			if j.webhook.Id == nil {
				errCh <- fmt.Errorf("webhook ID is required for update in %s/%s", j.repo.ProjectKey, j.repo.RepositorySlug)
				continue
			}
			updated, httpResp, err := c.api.RepositoryAPI.
				UpdateWebhook1(c.authCtx, j.repo.ProjectKey, utils.Int32PtrToString(j.webhook.Id), j.repo.RepositorySlug).
				RestWebhook(j.webhook).
				Execute()

			if err != nil {
				c.logger.Debug("details", "httpResp", httpResp)
				errCh <- fmt.Errorf("failed to update webhook %s in %s/%s: %w", utils.Int32PtrToString(j.webhook.Id), j.repo.ProjectKey, j.repo.RepositorySlug, err)
				continue
			}

			c.logger.Info("Updated webhook",
				"project", j.repo.ProjectKey,
				"repo", j.repo.RepositorySlug,
				"id", utils.SafeValue(updated.Id),
				"name", utils.SafeValue(updated.Name),
				"url", utils.SafeValue(updated.Url))
		}
	}

	// start worker pool
	wg.Add(maxWorkers)
	for i := 0; i < maxWorkers; i++ {
		go worker()
	}

	// send all jobs to the channel
	for _, r := range repos {
		for _, wh := range *r.Webhooks {
			jobs <- job{repo: r, webhook: wh}
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
		return fmt.Errorf("errors occurred updating webhooks: %s", strings.Join(errs, "; "))
	}

	return nil
}

// DeleteWebhook deletes all webhooks listed in repos.Webhooks concurrently.
func (c *Client) DeleteWebhooks(repos []models.ExtendedRepository) error {
	maxWorkers := config.GlobalMaxWorkers

	type job struct {
		repo    models.ExtendedRepository
		webhook openapi.RestWebhook
	}

	// count total tasks
	var total int
	for _, r := range repos {
		total += len(*r.Webhooks)
	}

	jobs := make(chan job, total)
	errCh := make(chan error, total)

	var wg sync.WaitGroup

	worker := func() {
		defer wg.Done()
		for j := range jobs {
			if j.webhook.Id == nil {
				c.logger.Error("Delete webhook failed",
					"project", j.repo.ProjectKey,
					"repo", j.repo.RepositorySlug,
					"id", nil,
					"error", "webhook ID is required")
				errCh <- fmt.Errorf("delete webhook failed: missing id")
				continue
			}

			httpResp, err := c.api.RepositoryAPI.
				DeleteWebhook1(c.authCtx, j.repo.ProjectKey, utils.Int32PtrToString(j.webhook.Id), j.repo.RepositorySlug).
				Execute()
			if err != nil {
				c.logger.Error("Delete webhook failed",
					"project", j.repo.ProjectKey,
					"repo", j.repo.RepositorySlug,
					"id", utils.Int32PtrToString(j.webhook.Id),
					"error", err)
				c.logger.Debug("HTTP response",
					"status", httpResp.Status,
					"statusCode", httpResp.StatusCode,
					"body", httpResp.Body)
				errCh <- err
				continue
			}

			c.logger.Info("Deleted webhook",
				"project", j.repo.ProjectKey,
				"repo", j.repo.RepositorySlug,
				"id", utils.Int32PtrToString(j.webhook.Id))
		}
	}

	// start worker pool
	wg.Add(maxWorkers)
	for i := 0; i < maxWorkers; i++ {
		go worker()
	}

	// send all jobs to the channel
	for _, r := range repos {
		for _, wh := range *r.Webhooks {
			jobs <- job{repo: r, webhook: wh}
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
		return fmt.Errorf("errors occurred deleting webhooks")
	}
	return nil
}

// UpsertWebhookByName ensures that a webhook with the given name exists.
// If a webhook with the same name already exists, it updates it, otherwise it creates a new one.
// func (c *Client) UpsertWebhookByName(w models.ExtendedRestWebhook) (*openapi.RestWebhook, error) {
// 	if w.Webhook.Name == nil || *w.Webhook.Name == "" {
// 		return nil, fmt.Errorf("webhook name is required for upsert")
// 	}

// 	// 1. Gte webhooks list
// 	existingHooks, err := c.FindWebhooks(w.ProjectKey, w.RepositorySlug)
// 	if err != nil {
// 		return nil, fmt.Errorf("failed to fetch webhooks for %s/%s: %w", w.ProjectKey, w.RepositorySlug, err)
// 	}

// 	// 2. Search by name
// 	for _, h := range existingHooks {
// 		if h.Name != nil && *h.Name == *w.Webhook.Name {
// 			// when found
// 			return c.UpdateWebhook(w)
// 		}
// 	}

// 	// 3. when not found
// 	return c.CreateWebhook(w)
// }
