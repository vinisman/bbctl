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

func (c *Client) GetWebhooks(projectKey, repoSlug string) ([]openapi.RestWebhook, error) {
	httpResp, err := c.api.RepositoryAPI.FindWebhooks1(c.authCtx, projectKey, repoSlug).Execute()
	if err != nil {
		return nil, fmt.Errorf("failed to call FindWebhooks1: %w", err)
	}
	defer httpResp.Body.Close()

	if httpResp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(httpResp.Body)
		return nil, fmt.Errorf("unexpected status %d: %s", httpResp.StatusCode, string(bodyBytes))
	}

	bodyBytes, err := io.ReadAll(httpResp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	c.logger.Debug("Details", "bodyBytes", bodyBytes)

	var webhooksResp models.WebhookResponse
	if err := json.Unmarshal(bodyBytes, &webhooksResp); err != nil {
		return nil, fmt.Errorf("failed to parse webhook response JSON: %w", err)
	}

	return webhooksResp.Values, nil
}

// CreateWebhook creates new webhooks concurrently for multiple repositories
func (c *Client) CreateWebhooks(repos []models.ExtendedRepository) error {
	maxWorkers := config.GlobalMaxWorkers

	type job struct {
		repo    models.ExtendedRepository
		webhook openapi.RestWebhook
	}

	// count the total number of webhook tasks
	var total int
	for _, r := range repos {
		total += len(r.Webhooks)
	}

	jobs := make(chan job, total)
	errCh := make(chan error, total)

	var wg sync.WaitGroup

	worker := func() {
		defer wg.Done()
		for j := range jobs {
			created, httpResp, err := c.api.RepositoryAPI.
				CreateWebhook1(c.authCtx, j.repo.ProjectKey, j.repo.RepositorySlug).
				RestWebhook(j.webhook).
				Execute()

			if err != nil {
				c.logger.Debug("details", "httpResp", httpResp)
				errCh <- fmt.Errorf("failed to create webhook %s in %s/%s: %w",
					utils.SafeString(j.webhook.Name),
					j.repo.ProjectKey, j.repo.RepositorySlug, err)
				continue
			}

			c.logger.Info("Created webhook",
				"project", j.repo.ProjectKey,
				"repo", j.repo.RepositorySlug,
				"id", utils.SafeInt32(created.Id),
				"name", utils.SafeString(created.Name),
				"url", utils.SafeString(created.Url))
		}
	}

	// start worker pool
	wg.Add(maxWorkers)
	for i := 0; i < maxWorkers; i++ {
		go worker()
	}

	// send all jobs to the channel
	for _, r := range repos {
		for _, wh := range r.Webhooks {
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
		return fmt.Errorf("errors occurred creating webhooks: %s", strings.Join(errs, "; "))
	}

	return nil
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
		total += len(r.Webhooks)
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
				"id", utils.SafeInt32(updated.Id),
				"name", utils.SafeString(updated.Name),
				"url", utils.SafeString(updated.Url))
		}
	}

	// start worker pool
	wg.Add(maxWorkers)
	for i := 0; i < maxWorkers; i++ {
		go worker()
	}

	// send all jobs to the channel
	for _, r := range repos {
		for _, wh := range r.Webhooks {
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
		total += len(r.Webhooks)
	}

	jobs := make(chan job, total)
	errCh := make(chan error, total)

	var wg sync.WaitGroup

	worker := func() {
		defer wg.Done()
		for j := range jobs {
			if j.webhook.Id == nil {
				errCh <- fmt.Errorf("webhook ID is required for deletion in %s/%s", j.repo.ProjectKey, j.repo.RepositorySlug)
				continue
			}
			httpResp, err := c.api.RepositoryAPI.
				DeleteWebhook1(c.authCtx, j.repo.ProjectKey, utils.Int32PtrToString(j.webhook.Id), j.repo.RepositorySlug).
				Execute()
			if err != nil {
				c.logger.Debug("details", "httpResp", httpResp)
				errCh <- fmt.Errorf("failed to delete webhook %s in %s/%s: %w", utils.Int32PtrToString(j.webhook.Id), j.repo.ProjectKey, j.repo.RepositorySlug, err)
				continue
			}
			c.logger.Info("Deleted webhook",
				"project", j.repo.ProjectKey,
				"repo", j.repo.RepositorySlug,
				"id", j.webhook.Id)
		}
	}

	// start worker pool
	wg.Add(maxWorkers)
	for i := 0; i < maxWorkers; i++ {
		go worker()
	}

	// send all jobs to the channel
	for _, r := range repos {
		for _, wh := range r.Webhooks {
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
		return fmt.Errorf("errors occurred deleting webhooks: %s", strings.Join(errs, "; "))
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
