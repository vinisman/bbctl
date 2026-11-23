package bitbucket

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/vinisman/bbctl/internal/models"
)

// projectUserPage returns one user permission page for a project.
func projectUserPage(name, permission string, nextPageStart *int) string {
	next := "null"
	if nextPageStart != nil {
		next = strconv.Itoa(*nextPageStart)
	}
	return `{"size":1,"limit":25,"isLastPage":` + strconv.FormatBool(nextPageStart == nil) +
		`,"start":0,"nextPageStart":` + next + `,"values":[{"permission":"` + permission +
		`","user":{"name":"` + name + `"}}]}`
}

func projectGroupPage(name, permission string) string {
	return `{"size":1,"limit":25,"isLastPage":true,"start":0,"values":[{"permission":"` + permission +
		`","group":{"name":"` + name + `"}}]}`
}

func repoUserPage(name, permission string) string {
	return `{"size":1,"limit":25,"isLastPage":true,"start":0,"values":[{"permission":"` + permission +
		`","user":{"name":"` + name + `"}}]}`
}

func repoGroupPage(name, permission string) string {
	return `{"size":1,"limit":25,"isLastPage":true,"start":0,"values":[{"permission":"` + permission +
		`","group":{"name":"` + name + `"}}]}`
}

// GetProjectPermissions must follow pagination and return users and groups.
func TestGetProjectPermissions_Paginated(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.HasSuffix(r.URL.Path, "/permissions/users"):
			if r.URL.Query().Get("start") == "0" || r.URL.Query().Get("start") == "" {
				next := 1
				_, _ = w.Write([]byte(projectUserPage("user1", "PROJECT_READ", &next)))
				return
			}
			_, _ = w.Write([]byte(projectUserPage("user2", "PROJECT_WRITE", nil)))
		case strings.HasSuffix(r.URL.Path, "/permissions/groups"):
			_, _ = w.Write([]byte(projectGroupPage("devs", "PROJECT_ADMIN")))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer ts.Close()

	c := newTestClient(t, ts.URL)
	c.config.PageSize = 25

	rows, err := c.GetProjectPermissions([]models.ProjectPermissions{{ProjectKey: "PK"}})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(rows) != 3 {
		t.Fatalf("got %d rows, want 3: %+v", len(rows), rows)
	}

	got := map[string]string{}
	for _, r := range rows {
		got[r.Type+":"+r.Name] = r.Permission
		if r.ProjectKey != "PK" {
			t.Fatalf("unexpected project key: %+v", r)
		}
	}
	want := map[string]string{
		"user:user1": "PROJECT_READ",
		"user:user2": "PROJECT_WRITE",
		"group:devs": "PROJECT_ADMIN",
	}
	for k, v := range want {
		if got[k] != v {
			t.Fatalf("row %s: got %q, want %q", k, got[k], v)
		}
	}
}

// GetRepositoryPermissions must query repo-scoped endpoints and keep the slug.
func TestGetRepositoryPermissions_UsersAndGroups(t *testing.T) {
	var mu sync.Mutex
	paths := map[string]int{}

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		paths[r.URL.Path]++
		mu.Unlock()

		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.HasSuffix(r.URL.Path, "/permissions/users"):
			_, _ = w.Write([]byte(repoUserPage("user1", "REPO_READ")))
		case strings.HasSuffix(r.URL.Path, "/permissions/groups"):
			_, _ = w.Write([]byte(repoGroupPage("devs", "REPO_WRITE")))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer ts.Close()

	c := newTestClient(t, ts.URL)
	c.config.PageSize = 25

	rows, err := c.GetRepositoryPermissions([]models.RepositoryPermissions{
		{ProjectKey: "PK", RepositorySlug: "r1"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("got %d rows, want 2: %+v", len(rows), rows)
	}
	for _, r := range rows {
		if r.ProjectKey != "PK" || r.RepositorySlug != "r1" {
			t.Fatalf("unexpected scope: %+v", r)
		}
	}

	wantPaths := []string{
		"/api/latest/projects/PK/repos/r1/permissions/users",
		"/api/latest/projects/PK/repos/r1/permissions/groups",
	}
	for _, p := range wantPaths {
		if paths[p] != 1 {
			t.Fatalf("path %s requested %d times, want 1", p, paths[p])
		}
	}
}

// GrantProjectPermissions must send one PUT per user and per group with name and permission.
func TestGrantProjectPermissions_SendsRequests(t *testing.T) {
	type call struct {
		method     string
		path       string
		name       string
		permission string
	}
	var mu sync.Mutex
	var calls []call

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		calls = append(calls, call{
			method:     r.Method,
			path:       r.URL.Path,
			name:       r.URL.Query().Get("name"),
			permission: r.URL.Query().Get("permission"),
		})
		mu.Unlock()
		w.WriteHeader(http.StatusNoContent)
	}))
	defer ts.Close()

	c := newTestClient(t, ts.URL)

	err := c.GrantProjectPermissions([]models.ProjectPermissions{{
		ProjectKey: "PK",
		Users:      []models.PermissionEntry{{Name: "user1", Permission: "PROJECT_READ"}},
		Groups:     []models.PermissionEntry{{Name: "devs", Permission: "PROJECT_WRITE"}},
	}})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(calls) != 2 {
		t.Fatalf("got %d calls, want 2: %+v", len(calls), calls)
	}

	byPath := map[string]call{}
	for _, cl := range calls {
		if cl.method != http.MethodPut {
			t.Fatalf("unexpected method %s", cl.method)
		}
		byPath[cl.path] = cl
	}

	u, ok := byPath["/api/latest/projects/PK/permissions/users"]
	if !ok || u.name != "user1" || u.permission != "PROJECT_READ" {
		t.Fatalf("unexpected users call: %+v", u)
	}
	g, ok := byPath["/api/latest/projects/PK/permissions/groups"]
	if !ok || g.name != "devs" || g.permission != "PROJECT_WRITE" {
		t.Fatalf("unexpected groups call: %+v", g)
	}
}

// RevokeRepositoryPermissions must send one DELETE per user and per group.
func TestRevokeRepositoryPermissions_SendsRequests(t *testing.T) {
	type call struct {
		method string
		path   string
		name   string
	}
	var mu sync.Mutex
	var calls []call

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		calls = append(calls, call{
			method: r.Method,
			path:   r.URL.Path,
			name:   r.URL.Query().Get("name"),
		})
		mu.Unlock()
		w.WriteHeader(http.StatusNoContent)
	}))
	defer ts.Close()

	c := newTestClient(t, ts.URL)

	err := c.RevokeRepositoryPermissions([]models.RepositoryPermissions{{
		ProjectKey:     "PK",
		RepositorySlug: "r1",
		Users:          []models.PermissionEntry{{Name: "user1"}},
		Groups:         []models.PermissionEntry{{Name: "devs"}},
	}})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(calls) != 2 {
		t.Fatalf("got %d calls, want 2: %+v", len(calls), calls)
	}

	for _, cl := range calls {
		if cl.method != http.MethodDelete {
			t.Fatalf("unexpected method %s", cl.method)
		}
		if !strings.HasPrefix(cl.path, "/api/latest/projects/PK/repos/r1/permissions/") {
			t.Fatalf("unexpected path %s", cl.path)
		}
		if cl.name != "user1" && cl.name != "devs" {
			t.Fatalf("unexpected name %s", cl.name)
		}
	}
}

// A failing users endpoint must not hide permissions fetched for other scopes.
func TestGetProjectPermissions_CollectsErrors(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		switch {
		case r.URL.Path == "/api/latest/projects/PK1/permissions/users":
			w.WriteHeader(http.StatusInternalServerError)
		case strings.HasSuffix(r.URL.Path, "/permissions/users"):
			_, _ = w.Write([]byte(repoUserPage("user2", "PROJECT_READ")))
		case strings.HasSuffix(r.URL.Path, "/permissions/groups"):
			_, _ = w.Write([]byte(repoGroupPage("devs", "PROJECT_WRITE")))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer ts.Close()

	c := newTestClient(t, ts.URL)
	c.config.PageSize = 25

	rows, err := c.GetProjectPermissions([]models.ProjectPermissions{
		{ProjectKey: "PK1"},
		{ProjectKey: "PK2"},
	})
	if err == nil {
		t.Fatal("expected error for failing users endpoint")
	}
	if !strings.Contains(err.Error(), "PK1") || !strings.Contains(err.Error(), "user permissions") {
		t.Fatalf("error should mention failing scope, got: %v", err)
	}
	if len(rows) != 3 {
		t.Fatalf("got %d rows, want 3: %+v", len(rows), rows)
	}
}

// Repository permissions must follow pagination as well.
func TestGetRepositoryPermissions_Paginated(t *testing.T) {
	var mu sync.Mutex
	starts := map[string]int{}

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		start := r.URL.Query().Get("start")
		mu.Lock()
		starts[start]++
		mu.Unlock()

		switch {
		case strings.HasSuffix(r.URL.Path, "/permissions/users"):
			if start == "0" || start == "" {
				next := 1
				_, _ = w.Write([]byte(projectUserPage("user1", "REPO_READ", &next)))
				return
			}
			_, _ = w.Write([]byte(projectUserPage("user2", "REPO_WRITE", nil)))
		case strings.HasSuffix(r.URL.Path, "/permissions/groups"):
			_, _ = w.Write([]byte(projectGroupPage("devs", "REPO_ADMIN")))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer ts.Close()

	c := newTestClient(t, ts.URL)
	c.config.PageSize = 25

	rows, err := c.GetRepositoryPermissions([]models.RepositoryPermissions{
		{ProjectKey: "PK", RepositorySlug: "r1"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(rows) != 3 {
		t.Fatalf("got %d rows, want 3: %+v", len(rows), rows)
	}
	if starts["0"] != 2 || starts["1"] != 1 {
		t.Fatalf("unexpected page starts: %+v", starts)
	}
}

// A failing users request must not prevent group permissions from being granted.
func TestGrantProjectPermissions_CollectsErrors(t *testing.T) {
	var mu sync.Mutex
	calls := map[string]int{}

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		calls[r.URL.Path]++
		mu.Unlock()

		if strings.HasSuffix(r.URL.Path, "/permissions/users") {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer ts.Close()

	c := newTestClient(t, ts.URL)

	err := c.GrantProjectPermissions([]models.ProjectPermissions{{
		ProjectKey: "PK",
		Users:      []models.PermissionEntry{{Name: "user1", Permission: "PROJECT_READ"}},
		Groups:     []models.PermissionEntry{{Name: "devs", Permission: "PROJECT_WRITE"}},
	}})
	if err == nil {
		t.Fatal("expected error for failing users endpoint")
	}
	if !strings.Contains(err.Error(), "user1") {
		t.Fatalf("error should mention failing user, got: %v", err)
	}
	if calls["/api/latest/projects/PK/permissions/users"] != 1 {
		t.Fatalf("users request not attempted: %+v", calls)
	}
	if calls["/api/latest/projects/PK/permissions/groups"] != 1 {
		t.Fatalf("groups request not attempted: %+v", calls)
	}
}

// RevokeProjectPermissions must send one DELETE per user and per group.
func TestRevokeProjectPermissions_SendsRequests(t *testing.T) {
	type call struct {
		method string
		path   string
		name   string
	}
	var mu sync.Mutex
	var calls []call

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		calls = append(calls, call{
			method: r.Method,
			path:   r.URL.Path,
			name:   r.URL.Query().Get("name"),
		})
		mu.Unlock()
		w.WriteHeader(http.StatusNoContent)
	}))
	defer ts.Close()

	c := newTestClient(t, ts.URL)

	err := c.RevokeProjectPermissions([]models.ProjectPermissions{{
		ProjectKey: "PK",
		Users:      []models.PermissionEntry{{Name: "user1"}},
		Groups:     []models.PermissionEntry{{Name: "devs"}},
	}})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(calls) != 2 {
		t.Fatalf("got %d calls, want 2: %+v", len(calls), calls)
	}

	names := map[string]bool{}
	for _, cl := range calls {
		if cl.method != http.MethodDelete {
			t.Fatalf("unexpected method %s", cl.method)
		}
		if !strings.HasPrefix(cl.path, "/api/latest/projects/PK/permissions/") {
			t.Fatalf("unexpected path %s", cl.path)
		}
		names[cl.name] = true
	}
	if !names["user1"] || !names["devs"] {
		t.Fatalf("unexpected names: %+v", names)
	}
}

// GrantRepositoryPermissions must send repo-scoped PUT requests with name and permission.
func TestGrantRepositoryPermissions_SendsRequests(t *testing.T) {
	type call struct {
		method     string
		path       string
		name       string
		permission string
	}
	var mu sync.Mutex
	var calls []call

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		calls = append(calls, call{
			method:     r.Method,
			path:       r.URL.Path,
			name:       r.URL.Query().Get("name"),
			permission: r.URL.Query().Get("permission"),
		})
		mu.Unlock()
		w.WriteHeader(http.StatusNoContent)
	}))
	defer ts.Close()

	c := newTestClient(t, ts.URL)

	err := c.GrantRepositoryPermissions([]models.RepositoryPermissions{{
		ProjectKey:     "PK",
		RepositorySlug: "r1",
		Users:          []models.PermissionEntry{{Name: "user1", Permission: "REPO_READ"}},
		Groups:         []models.PermissionEntry{{Name: "devs", Permission: "REPO_ADMIN"}},
	}})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(calls) != 2 {
		t.Fatalf("got %d calls, want 2: %+v", len(calls), calls)
	}

	byPath := map[string]call{}
	for _, cl := range calls {
		if cl.method != http.MethodPut {
			t.Fatalf("unexpected method %s", cl.method)
		}
		byPath[cl.path] = cl
	}

	u, ok := byPath["/api/latest/projects/PK/repos/r1/permissions/users"]
	if !ok || u.name != "user1" || u.permission != "REPO_READ" {
		t.Fatalf("unexpected users call: %+v", u)
	}
	g, ok := byPath["/api/latest/projects/PK/repos/r1/permissions/groups"]
	if !ok || g.name != "devs" || g.permission != "REPO_ADMIN" {
		t.Fatalf("unexpected groups call: %+v", g)
	}
}

// Requests to different projects run in parallel and the output order matches
// the input order.
func TestGetProjectPermissions_RunsConcurrently(t *testing.T) {
	const n = 10

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
		if strings.HasSuffix(r.URL.Path, "/permissions/users") {
			// /api/latest/projects/{projectKey}/permissions/users
			p := strings.TrimSuffix(r.URL.Path, "/permissions/users")
			key := p[strings.LastIndex(p, "/")+1:]
			_, _ = w.Write([]byte(projectUserPage(key, "PROJECT_READ", nil)))
			return
		}
		_, _ = w.Write([]byte(`{"size":0,"limit":25,"isLastPage":true,"start":0,"values":[]}`))
	}))
	defer ts.Close()

	setWorkers(t, 5)
	c := newTestClient(t, ts.URL)
	c.config.PageSize = 25

	projects := make([]models.ProjectPermissions, n)
	for i := range projects {
		projects[i] = models.ProjectPermissions{ProjectKey: fmt.Sprintf("PK%02d", i)}
	}

	rows, err := c.GetProjectPermissions(projects)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := atomic.LoadInt32(&max); got < 2 {
		t.Fatalf("requests were sequential: max concurrency = %d", got)
	}
	if len(rows) != n {
		t.Fatalf("got %d rows, want %d", len(rows), n)
	}
	for i, r := range rows {
		if r.ProjectKey != projects[i].ProjectKey {
			t.Fatalf("order broken at index %d: got %s, want %s", i, r.ProjectKey, projects[i].ProjectKey)
		}
	}
}

// Grants across repositories run in parallel.
func TestGrantRepositoryPermissions_RunsConcurrently(t *testing.T) {
	const n = 10

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
		w.WriteHeader(http.StatusNoContent)
	}))
	defer ts.Close()

	setWorkers(t, 5)
	c := newTestClient(t, ts.URL)

	repos := make([]models.RepositoryPermissions, n)
	for i := range repos {
		repos[i] = models.RepositoryPermissions{
			ProjectKey:     "PK",
			RepositorySlug: fmt.Sprintf("r%02d", i),
			Users:          []models.PermissionEntry{{Name: "user1", Permission: "REPO_READ"}},
		}
	}

	if err := c.GrantRepositoryPermissions(repos); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := atomic.LoadInt32(&max); got < 2 {
		t.Fatalf("requests were sequential: max concurrency = %d", got)
	}
}

// Users with the same permission in one repository are sent in a single request.
func TestGrantRepositoryPermissions_BatchesByName(t *testing.T) {
	var mu sync.Mutex
	calls := 0
	namesByPermission := map[string][]string{}

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		calls++
		namesByPermission[r.URL.Query().Get("permission")] = append(
			namesByPermission[r.URL.Query().Get("permission")], r.URL.Query()["name"]...)
		mu.Unlock()
		w.WriteHeader(http.StatusNoContent)
	}))
	defer ts.Close()

	c := newTestClient(t, ts.URL)

	err := c.GrantRepositoryPermissions([]models.RepositoryPermissions{{
		ProjectKey:     "PK",
		RepositorySlug: "r1",
		Users: []models.PermissionEntry{
			{Name: "u1", Permission: "REPO_READ"},
			{Name: "u2", Permission: "REPO_READ"},
			{Name: "u3", Permission: "REPO_WRITE"},
		},
	}})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if calls != 2 {
		t.Fatalf("got %d requests, want 2 (batched by permission)", calls)
	}
	if len(namesByPermission["REPO_READ"]) != 2 {
		t.Fatalf("REPO_READ batch: %+v", namesByPermission["REPO_READ"])
	}
	if len(namesByPermission["REPO_WRITE"]) != 1 {
		t.Fatalf("REPO_WRITE batch: %+v", namesByPermission["REPO_WRITE"])
	}
}
