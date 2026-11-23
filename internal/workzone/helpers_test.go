package workzone

import (
	"errors"
	"fmt"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/vinisman/bbctl/internal/config"
	"github.com/vinisman/bbctl/internal/models"
	wz "github.com/vinisman/workzone-sdk-go/client"
)

func setWorkers(t *testing.T, n int) {
	t.Helper()
	prev := config.GlobalMaxWorkers
	config.GlobalMaxWorkers = n
	t.Cleanup(func() { config.GlobalMaxWorkers = prev })
}

func testRepos(n int) []models.ExtendedRepository {
	repos := make([]models.ExtendedRepository, n)
	for i := range repos {
		repos[i] = models.ExtendedRepository{ProjectKey: "PK", RepositorySlug: fmt.Sprintf("r%02d", i)}
	}
	return repos
}

func TestBatchOperation_Empty(t *testing.T) {
	setWorkers(t, 2)

	if err := batchOperation(nil, func(models.ExtendedRepository) error {
		t.Fatal("operation must not be called")
		return nil
	}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestBatchOperation_AllSucceed(t *testing.T) {
	setWorkers(t, 4)
	repos := testRepos(10)

	var count int32
	err := batchOperation(repos, func(models.ExtendedRepository) error {
		atomic.AddInt32(&count, 1)
		return nil
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if count != int32(len(repos)) {
		t.Fatalf("operation calls = %d, want %d", count, len(repos))
	}
}

func TestBatchOperation_CollectsErrors(t *testing.T) {
	setWorkers(t, 4)
	repos := testRepos(5)

	err := batchOperation(repos, func(r models.ExtendedRepository) error {
		if r.RepositorySlug == "r02" {
			return fmt.Errorf("%s: boom", r.RepositorySlug)
		}
		return nil
	})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "r02: boom") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestBatchGetOperation_PreservesOrder(t *testing.T) {
	setWorkers(t, 4)
	repos := testRepos(8)

	out, err := batchGetOperation(repos,
		func(projectKey, repoSlug string) ([]wz.RestBranchReviewers, error) {
			return []wz.RestBranchReviewers{{}}, nil
		},
		func(w *models.WorkzoneData, v []wz.RestBranchReviewers) {
			w.Reviewers = v
		},
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(out) != len(repos) {
		t.Fatalf("repos = %d, want %d", len(out), len(repos))
	}
	for i, r := range out {
		if r.RepositorySlug != repos[i].RepositorySlug {
			t.Fatalf("order broken at %d: %q != %q", i, r.RepositorySlug, repos[i].RepositorySlug)
		}
		if r.Workzone == nil || len(r.Workzone.Reviewers) != 1 {
			t.Fatalf("workzone not populated for %s", r.RepositorySlug)
		}
	}
}

func TestBatchGetOperation_CollectsErrors(t *testing.T) {
	setWorkers(t, 4)
	repos := testRepos(5)

	out, err := batchGetOperation(repos,
		func(projectKey, repoSlug string) ([]wz.RestBranchReviewers, error) {
			if repoSlug == "r03" {
				return nil, errors.New("boom")
			}
			return []wz.RestBranchReviewers{{}}, nil
		},
		func(w *models.WorkzoneData, v []wz.RestBranchReviewers) {
			w.Reviewers = v
		},
	)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "PK/r03") {
		t.Fatalf("error should mention repo, got: %v", err)
	}
	if len(out) != len(repos) {
		t.Fatalf("repos = %d, want %d", len(out), len(repos))
	}
}

// A zero GlobalMaxWorkers (e.g. an unset or broken config) must not deadlock
// the semaphore: the pool falls back to a single worker.
func TestBatchOperation_ZeroWorkersDoesNotDeadlock(t *testing.T) {
	setWorkers(t, 0)
	repos := testRepos(5)

	done := make(chan error, 1)
	go func() {
		done <- batchOperation(repos, func(models.ExtendedRepository) error { return nil })
	}()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("batchOperation deadlocked with GlobalMaxWorkers=0")
	}
}

func TestBatchGetOperation_ZeroWorkersDoesNotDeadlock(t *testing.T) {
	setWorkers(t, 0)
	repos := testRepos(5)

	type res struct {
		out []models.ExtendedRepository
		err error
	}
	done := make(chan res, 1)
	go func() {
		out, err := batchGetOperation(repos,
			func(projectKey, repoSlug string) ([]wz.RestBranchReviewers, error) {
				return []wz.RestBranchReviewers{{}}, nil
			},
			func(w *models.WorkzoneData, v []wz.RestBranchReviewers) {
				w.Reviewers = v
			},
		)
		done <- res{out, err}
	}()

	select {
	case r := <-done:
		if r.err != nil {
			t.Fatalf("unexpected error: %v", r.err)
		}
		if len(r.out) != len(repos) {
			t.Fatalf("repos = %d, want %d", len(r.out), len(repos))
		}
	case <-time.After(5 * time.Second):
		t.Fatal("batchGetOperation deadlocked with GlobalMaxWorkers=0")
	}
}

func TestBatchGetOperation_Empty(t *testing.T) {
	setWorkers(t, 2)

	out, err := batchGetOperation(nil,
		func(projectKey, repoSlug string) ([]wz.RestBranchReviewers, error) {
			t.Fatal("operation must not be called")
			return nil, nil
		},
		func(w *models.WorkzoneData, v []wz.RestBranchReviewers) {},
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(out) != 0 {
		t.Fatalf("repos = %d, want 0", len(out))
	}
}
