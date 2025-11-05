package workzone

import (
	"errors"
	"fmt"
	"sync"

	"github.com/vinisman/bbctl/internal/config"
	"github.com/vinisman/bbctl/internal/models"
)

// batchOperation executes operation on multiple repositories concurrently
func batchOperation(
	repos []models.ExtendedRepository,
	operation func(models.ExtendedRepository) error,
) error {
	if len(repos) == 0 {
		return nil
	}

	maxWorkers := config.GlobalMaxWorkers
	sem := make(chan struct{}, maxWorkers)
	var wg sync.WaitGroup
	var mu sync.Mutex
	var errs []error

	for i := range repos {
		wg.Add(1)
		go func(r models.ExtendedRepository) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			if err := operation(r); err != nil {
				mu.Lock()
				errs = append(errs, err)
				mu.Unlock()
			}
		}(repos[i])
	}

	wg.Wait()

	if len(errs) > 0 {
		return errors.Join(errs...)
	}
	return nil
}

// batchGetOperation executes get operation on multiple repositories concurrently
// and populates the Workzone field of each repository
func batchGetOperation[T any](
	repos []models.ExtendedRepository,
	operation func(projectKey, repoSlug string) (T, error),
	setter func(*models.WorkzoneData, T),
) ([]models.ExtendedRepository, error) {
	if len(repos) == 0 {
		return repos, nil
	}

	maxWorkers := config.GlobalMaxWorkers
	sem := make(chan struct{}, maxWorkers)
	var wg sync.WaitGroup
	var mu sync.Mutex
	var errs []error

	out := make([]models.ExtendedRepository, len(repos))
	copy(out, repos)

	for i := range out {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			r := out[idx]
			result, err := operation(r.ProjectKey, r.RepositorySlug)
			if err != nil {
				mu.Lock()
				errs = append(errs, fmt.Errorf("%s/%s: %w", r.ProjectKey, r.RepositorySlug, err))
				mu.Unlock()
				return
			}

			mu.Lock()
			if out[idx].Workzone == nil {
				out[idx].Workzone = &models.WorkzoneData{}
			}
			setter(out[idx].Workzone, result)
			mu.Unlock()
		}(i)
	}

	wg.Wait()

	if len(errs) > 0 {
		return out, errors.Join(errs...)
	}

	return out, nil
}
