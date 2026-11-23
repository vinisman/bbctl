package bitbucket

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/vinisman/bbctl/internal/config"
	"github.com/vinisman/bbctl/internal/models"
	openapi "github.com/vinisman/bitbucket-sdk-go/openapi"
)

// roundTripFunc lets benchmarks answer requests in-process: no sockets, so long
// runs cannot exhaust ephemeral ports on macOS.
type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

// permissionRoundTripper mimics the Bitbucket permission endpoints.
func permissionRoundTripper() roundTripFunc {
	return func(r *http.Request) (*http.Response, error) {
		header := make(http.Header)
		header.Set("Content-Type", "application/json")

		if r.Method == http.MethodGet {
			body := `{"size":1,"limit":25,"isLastPage":true,"start":0,"values":[` +
				`{"permission":"PROJECT_ADMIN","group":{"name":"devs"}}]}`
			if strings.HasSuffix(r.URL.Path, "/permissions/users") {
				body = `{"size":2,"limit":25,"isLastPage":true,"start":0,"values":[` +
					`{"permission":"PROJECT_READ","user":{"name":"user1"}},` +
					`{"permission":"PROJECT_WRITE","user":{"name":"user2"}}]}`
			}
			return &http.Response{
				StatusCode: http.StatusOK,
				Status:     "200 OK",
				Header:     header,
				Body:       io.NopCloser(strings.NewReader(body)),
				Request:    r,
			}, nil
		}

		return &http.Response{
			StatusCode: http.StatusNoContent,
			Status:     "204 No Content",
			Header:     header,
			Body:       io.NopCloser(strings.NewReader("")),
			Request:    r,
		}, nil
	}
}

// newInProcessClient builds a Client backed by the in-process transport. It is
// shared by benchmarks and the leak test: no sockets means no ephemeral port
// exhaustion on long runs.
func newInProcessClient(tb testing.TB) *Client {
	tb.Helper()

	cfg := openapi.NewConfiguration()
	cfg.Servers = openapi.ServerConfigurations{{URL: "http://bench.local"}}
	cfg.HTTPClient = &http.Client{Transport: permissionRoundTripper()}
	api := openapi.NewAPIClient(cfg)

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	return &Client{
		api:     api,
		logger:  logger,
		authCtx: context.Background(),
		config:  &config.Config{BaseURL: "http://bench.local", PageSize: 25},
		Logger:  logger,
	}
}

// newBenchClient mirrors newInProcessClient but accepts *testing.B.
func newBenchClient(b *testing.B) *Client {
	b.Helper()
	return newInProcessClient(b)
}

func benchProjects(n int) []models.ProjectPermissions {
	projects := make([]models.ProjectPermissions, n)
	for i := range projects {
		projects[i] = models.ProjectPermissions{ProjectKey: fmt.Sprintf("PK%02d", i)}
	}
	return projects
}

func benchRepos(n int) []models.RepositoryPermissions {
	repos := make([]models.RepositoryPermissions, n)
	for i := range repos {
		repos[i] = models.RepositoryPermissions{
			ProjectKey:     "PK",
			RepositorySlug: fmt.Sprintf("r%02d", i),
			Users: []models.PermissionEntry{
				{Name: "user1", Permission: "REPO_READ"},
				{Name: "user2", Permission: "REPO_READ"},
			},
			Groups: []models.PermissionEntry{{Name: "devs", Permission: "REPO_WRITE"}},
		}
	}
	return repos
}

func benchProjectsWithPermissions(n int) []models.ProjectPermissions {
	projects := make([]models.ProjectPermissions, n)
	for i := range projects {
		projects[i] = models.ProjectPermissions{
			ProjectKey: fmt.Sprintf("PK%02d", i),
			Users:      []models.PermissionEntry{{Name: "user1", Permission: "PROJECT_READ"}},
			Groups:     []models.PermissionEntry{{Name: "devs", Permission: "PROJECT_WRITE"}},
		}
	}
	return projects
}

func setBenchWorkers(b *testing.B, n int) {
	b.Helper()
	prev := config.GlobalMaxWorkers
	config.GlobalMaxWorkers = n
	b.Cleanup(func() { config.GlobalMaxWorkers = prev })
}

func BenchmarkGetProjectPermissions(b *testing.B) {
	setBenchWorkers(b, 5)
	c := newBenchClient(b)
	projects := benchProjects(10)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := c.GetProjectPermissions(projects); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkGetRepositoryPermissions(b *testing.B) {
	setBenchWorkers(b, 5)
	c := newBenchClient(b)
	repos := benchRepos(10)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := c.GetRepositoryPermissions(repos); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkGrantRepositoryPermissions(b *testing.B) {
	setBenchWorkers(b, 5)
	c := newBenchClient(b)
	repos := benchRepos(10)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := c.GrantRepositoryPermissions(repos); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkRevokeRepositoryPermissions(b *testing.B) {
	setBenchWorkers(b, 5)
	c := newBenchClient(b)
	repos := benchRepos(10)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := c.RevokeRepositoryPermissions(repos); err != nil {
			b.Fatal(err)
		}
	}
}

// collectGarbage runs GC so goroutine and heap measurements are stable.
func collectGarbage() {
	runtime.GC()
}

// TestPermissionOperations_NoLeaks runs the full permission cycle many times and
// checks that goroutines and heap do not grow. Requests are served in-process so
// the test measures the client, not the OS socket table.
func TestPermissionOperations_NoLeaks(t *testing.T) {
	setWorkers(t, 5)
	c := newInProcessClient(t)
	c.config.PageSize = 25

	projects := benchProjects(10)
	projectsWithPerms := benchProjectsWithPermissions(10)
	repos := benchRepos(10)

	run := func() {
		if _, err := c.GetProjectPermissions(projects); err != nil {
			t.Fatalf("get project permissions: %v", err)
		}
		if _, err := c.GetRepositoryPermissions(repos); err != nil {
			t.Fatalf("get repository permissions: %v", err)
		}
		if err := c.GrantProjectPermissions(projectsWithPerms); err != nil {
			t.Fatalf("grant project permissions: %v", err)
		}
		if err := c.RevokeProjectPermissions(projectsWithPerms); err != nil {
			t.Fatalf("revoke project permissions: %v", err)
		}
		if err := c.GrantRepositoryPermissions(repos); err != nil {
			t.Fatalf("grant repository permissions: %v", err)
		}
		if err := c.RevokeRepositoryPermissions(repos); err != nil {
			t.Fatalf("revoke repository permissions: %v", err)
		}
	}

	// Warm up so one-time allocations and connection setup are not counted.
	for i := 0; i < 5; i++ {
		run()
	}

	time.Sleep(50 * time.Millisecond)
	collectGarbage()

	var before runtime.MemStats
	runtime.ReadMemStats(&before)
	baseGoroutines := runtime.NumGoroutine()

	const iterations = 200
	for i := 0; i < iterations; i++ {
		run()
	}

	time.Sleep(100 * time.Millisecond)
	collectGarbage()

	var after runtime.MemStats
	runtime.ReadMemStats(&after)
	afterGoroutines := runtime.NumGoroutine()

	heapDelta := int64(after.HeapAlloc) - int64(before.HeapAlloc)
	t.Logf("goroutines: before=%d after=%d, heap delta: %d bytes", baseGoroutines, afterGoroutines, heapDelta)

	if afterGoroutines > baseGoroutines+2 {
		t.Fatalf("goroutine leak: before=%d after=%d", baseGoroutines, afterGoroutines)
	}

	const maxHeapGrowth = 8 << 20 // 8 MiB
	if heapDelta > maxHeapGrowth {
		t.Fatalf("possible memory leak: heap grew by %d bytes over %d iterations", heapDelta, iterations)
	}
}
