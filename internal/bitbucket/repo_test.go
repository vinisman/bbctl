package bitbucket

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/vinisman/bbctl/internal/config"
	"github.com/vinisman/bbctl/internal/models"
	openapi "github.com/vinisman/bitbucket-sdk-go/openapi"
)

// newTestClient builds a Client directly, bypassing NewClient env configuration.
// setWorkers sets the global worker count and restores it after the test.
func setWorkers(t *testing.T, n int) {
	t.Helper()
	prev := config.GlobalMaxWorkers
	config.GlobalMaxWorkers = n
	t.Cleanup(func() { config.GlobalMaxWorkers = prev })
}

func newTestClient(t *testing.T, serverURL string) *Client {
	t.Helper()

	cfg := openapi.NewConfiguration()
	cfg.Servers = openapi.ServerConfigurations{{URL: serverURL}}
	cfg.HTTPClient = &http.Client{}
	api := openapi.NewAPIClient(cfg)

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	return &Client{
		api:     api,
		logger:  logger,
		authCtx: context.Background(),
		config:  &config.Config{BaseURL: serverURL},
		Logger:  logger,
	}
}

// restrictionsResponse returns a /restrictions response with one permission for a repo.
func restrictionsResponse(slug string) []byte {
	return []byte(fmt.Sprintf(`{"size":1,"limit":25,"isLastPage":true,"start":0,"values":[{`+
		`"id":1,"type":"pull-request-only",`+
		`"matcher":{"displayId":"trunk","id":"refs/heads/trunk","type":{"id":"BRANCH","name":"Branch"}},`+
		`"scope":{"type":"REPOSITORY"},`+
		`"users":[{"name":"user-%s"}]}]}`, slug))
}

func testRepos(n int) []models.ExtendedRepository {
	repos := make([]models.ExtendedRepository, n)
	for i := range repos {
		repos[i] = models.ExtendedRepository{ProjectKey: "PK", RepositorySlug: fmt.Sprintf("r%02d", i)}
	}
	return repos
}

// All repos processed, output order matches input order, and permission data is filled.
func TestGetBranchPermissions_AllProcessedOrderPreserved(t *testing.T) {
	const n = 10

	var mu sync.Mutex
	seen := map[string]int{}

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// slug is the last path segment: .../repos/{slug}/restrictions
		p := strings.TrimSuffix(r.URL.Path, "/restrictions")
		slug := p[strings.LastIndex(p, "/")+1:]
		mu.Lock()
		seen[slug]++
		mu.Unlock()

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(restrictionsResponse(slug))
	}))
	defer ts.Close()

	setWorkers(t, 5)
	c := newTestClient(t, ts.URL)

	out, err := c.GetBranchPermissions(testRepos(n))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(out) != n {
		t.Fatalf("got %d repos, want %d", len(out), n)
	}

	for i, r := range out {
		wantSlug := fmt.Sprintf("r%02d", i)
		if r.RepositorySlug != wantSlug {
			t.Fatalf("order broken: index %d has slug %q, want %q", i, r.RepositorySlug, wantSlug)
		}
		if r.BranchPermissions == nil || len(*r.BranchPermissions) != 1 {
			t.Fatalf("repo %s: expected 1 permission", wantSlug)
		}
		p := (*r.BranchPermissions)[0]
		if p.Matcher == nil || p.Matcher.Id == nil || *p.Matcher.Id != "refs/heads/trunk" {
			t.Fatalf("repo %s: unexpected matcher", wantSlug)
		}
		if len(p.Users) != 1 || p.Users[0].Name == nil || *p.Users[0].Name != "user-"+wantSlug {
			t.Fatalf("repo %s: unexpected users", wantSlug)
		}
	}

	for i := 0; i < n; i++ {
		if seen[fmt.Sprintf("r%02d", i)] != 1 {
			t.Fatalf("repo r%02d requested %d times, want 1", i, seen[fmt.Sprintf("r%02d", i)])
		}
	}
}

// Requests run in parallel (worker pool is active).
func TestGetBranchPermissions_RunsConcurrently(t *testing.T) {
	const n = 20

	var current, max int32
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cur := atomic.AddInt32(&current, 1)
		for {
			m := atomic.LoadInt32(&max)
			if cur <= m || atomic.CompareAndSwapInt32(&max, m, cur) {
				break
			}
		}
		time.Sleep(100 * time.Millisecond)
		atomic.AddInt32(&current, -1)

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"size":0,"limit":25,"isLastPage":true,"start":0,"values":[]}`))
	}))
	defer ts.Close()

	setWorkers(t, 5)
	c := newTestClient(t, ts.URL)

	if _, err := c.GetBranchPermissions(testRepos(n)); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if got := atomic.LoadInt32(&max); got < 2 {
		t.Fatalf("requests were sequential: max concurrency = %d", got)
	}
}

// A single repo failure: the call returns an error mentioning the repo slug.
func TestGetBranchPermissions_CollectsError(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/repos/r05/") {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"size":0,"limit":25,"isLastPage":true,"start":0,"values":[]}`))
	}))
	defer ts.Close()

	setWorkers(t, 4)
	c := newTestClient(t, ts.URL)

	_, err := c.GetBranchPermissions(testRepos(10))
	if err == nil {
		t.Fatal("expected error for failing repo")
	}
	if !strings.Contains(err.Error(), "PK/r05") {
		t.Fatalf("error should mention failing repo, got: %v", err)
	}
}

// Empty restrictions: repos are returned with empty lists and no error.
func TestGetBranchPermissions_EmptyValues(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"size":0,"limit":25,"isLastPage":true,"start":0,"values":[]}`))
	}))
	defer ts.Close()

	setWorkers(t, 4)
	c := newTestClient(t, ts.URL)

	out, err := c.GetBranchPermissions(testRepos(3))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(out) != 3 {
		t.Fatalf("got %d repos, want 3", len(out))
	}
	for _, r := range out {
		if r.BranchPermissions == nil || len(*r.BranchPermissions) != 0 {
			t.Fatalf("repo %s: expected empty permissions", r.RepositorySlug)
		}
	}
}
