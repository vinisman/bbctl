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

		if httpResp.StatusCode != http.StatusOK {
			bodyBytes, readErr := io.ReadAll(httpResp.Body)
			httpResp.Body.Close()
			if readErr != nil {
				errs = append(errs, fmt.Sprintf("unexpected status %d for %s/%s: failed to read body: %v", httpResp.StatusCode, repos[i].ProjectKey, repos[i].RepositorySlug, readErr))
			} else {
				errs = append(errs, fmt.Sprintf("unexpected status %d for %s/%s: %s", httpResp.StatusCode, repos[i].ProjectKey, repos[i].RepositorySlug, string(bodyBytes)))
			}
			continue
		}

		bodyBytes, err := io.ReadAll(httpResp.Body)
		httpResp.Body.Close()
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

	type result struct {
		repoIndex int
		webhook   openapi.RestWebhook
	}

	// count the total number of webhook tasks
	var total int
	for _, r := range repos {
		total += len(*r.Webhooks)
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
				if httpResp != nil {
					c.logger.Debug("HTTP response", "status", httpResp.StatusCode, "body", httpResp.Body)
				}
				errCh <- err
				continue
			}

			resultsCh <- result{repoIndex: j.repoIndex, webhook: *created}

			c.logger.Info("Created webhook",
				"project", j.repo.ProjectKey,
				"repo", j.repo.RepositorySlug,
				"id", utils.SafeValue(created.Id),
				"name", utils.SafeValue(created.Name))
		}
	}

	// start worker pool
	wg.Add(maxWorkers)
	for range maxWorkers {
		go worker()
	}

	// send all jobs to the channel
	for i, r := range repos {
		for _, wh := range *r.Webhooks {
			jobs <- job{repoIndex: i, repo: r, webhook: wh}
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
		newRepos[i].Webhooks = &[]openapi.RestWebhook{}
	}

	for res := range resultsCh {
		*newRepos[res.repoIndex].Webhooks = append(*newRepos[res.repoIndex].Webhooks, res.webhook)
	}

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
func (c *Client) UpdateWebhooks(repos []models.ExtendedRepository) ([]models.ExtendedRepository, error) {
	maxWorkers := config.GlobalMaxWorkers

	type job struct {
		repoIndex int
		repo      models.ExtendedRepository
		webhook   openapi.RestWebhook
	}

	type result struct {
		repoIndex int
		webhook   openapi.RestWebhook
	}

	// count total tasks
	var total int
	for _, r := range repos {
		total += len(*r.Webhooks)
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
			if j.webhook.Id == nil {
				errCh <- fmt.Errorf("webhook ID is required for update in %s/%s", j.repo.ProjectKey, j.repo.RepositorySlug)
				continue
			}
			updated, httpResp, err := c.api.RepositoryAPI.
				UpdateWebhook1(c.authCtx, j.repo.ProjectKey, utils.Int32PtrToString(j.webhook.Id), j.repo.RepositorySlug).
				RestWebhook(j.webhook).
				Execute()

			if err != nil {
				if httpResp != nil {
					c.logger.Debug("HTTP response", "status", httpResp.StatusCode, "body", httpResp.Body)
					if httpResp.StatusCode == 404 {
						c.logger.Info("Webhook not found during update, skipping",
							"project", j.repo.ProjectKey,
							"repo", j.repo.RepositorySlug,
							"id", utils.Int32PtrToString(j.webhook.Id))
						continue
					}
				}
				errCh <- fmt.Errorf("failed to update webhook %s in %s/%s: %w", utils.Int32PtrToString(j.webhook.Id), j.repo.ProjectKey, j.repo.RepositorySlug, err)
				continue
			}

			resultsCh <- result{repoIndex: j.repoIndex, webhook: *updated}

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
	for range maxWorkers {
		go worker()
	}

	// send all jobs to the channel
	for i, r := range repos {
		for _, wh := range *r.Webhooks {
			jobs <- job{repoIndex: i, repo: r, webhook: wh}
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
		newRepos[i].Webhooks = &[]openapi.RestWebhook{}
	}

	for res := range resultsCh {
		*newRepos[res.repoIndex].Webhooks = append(*newRepos[res.repoIndex].Webhooks, res.webhook)
	}

	// collect first error
	var firstErr error
	for e := range errCh {
		if firstErr == nil {
			firstErr = e
		}
	}

	// filter newRepos: only those with at least one webhook
	updatedRepos := []models.ExtendedRepository{}
	for _, r := range newRepos {
		if len(*r.Webhooks) > 0 {
			updatedRepos = append(updatedRepos, r)
		}
	}

	return updatedRepos, firstErr
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
				if httpResp != nil {
					c.logger.Debug("HTTP response", "status", httpResp.StatusCode, "body", httpResp.Body)
					if httpResp.StatusCode == 404 {
						c.logger.Info("Webhook was already absent (404), skipping",
							"project", j.repo.ProjectKey,
							"repo", j.repo.RepositorySlug,
							"id", utils.Int32PtrToString(j.webhook.Id))
						continue
					}
				}
				c.logger.Error("Delete webhook failed",
					"project", j.repo.ProjectKey,
					"repo", j.repo.RepositorySlug,
					"id", utils.Int32PtrToString(j.webhook.Id),
					"error", err)
				errCh <- err
				continue
			}

			c.logger.Info("Deleted webhook (or was not present)",
				"project", j.repo.ProjectKey,
				"repo", j.repo.RepositorySlug,
				"id", utils.Int32PtrToString(j.webhook.Id))
		}
	}

	// start worker pool
	wg.Add(maxWorkers)
	for range maxWorkers {
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

// AreWebhooksEqual compares two webhooks by relevant fields.
// Credentials are intentionally ignored because they are not retrievable via GET and usually write-only.
func AreWebhooksEqual(a, b openapi.RestWebhook) bool {
	if !equalStringPtr(a.Name, b.Name) {
		return false
	}
	if !equalStringPtr(a.Url, b.Url) {
		return false
	}
	if !equalBoolPtr(a.Active, b.Active) {
		return false
	}
	if !equalStringPtr(a.ScopeType, b.ScopeType) {
		return false
	}
	if !equalBoolPtr(a.SslVerificationRequired, b.SslVerificationRequired) {
		return false
	}
	if !equalStringSlices(a.Events, b.Events) {
		return false
	}
	return true
}

func equalBoolPtr(a, b *bool) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	return *a == *b
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
