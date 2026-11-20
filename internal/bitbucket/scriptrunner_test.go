package bitbucket

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/vinisman/bbctl/internal/config"
	"github.com/vinisman/bbctl/internal/models"
)

// scriptrunnerTestSetup creates an httptest server with a bbctl client and silences logging.
// The auth header check verifies that the Bearer token is sent in raw requests.
func scriptrunnerTestSetup(t *testing.T, handler http.HandlerFunc) *Client {
	t.Helper()
	ts := httptest.NewServer(handler)
	t.Cleanup(ts.Close)

	// scriptrunner tests do not exercise retry — a single attempt so that
	// metadata failures (5xx) do not wait for backoff
	config.GlobalRetryMaxAttempts = 1
	config.GlobalRetryBaseDelayMs = 1

	t.Cleanup(func() {
		config.GlobalCfg = nil
		config.GlobalMaxWorkers = 5
		config.GlobalRetryMaxAttempts = 4
		config.GlobalRetryBaseDelayMs = 500
	})
	config.GlobalCfg = &config.Config{
		BaseURL: ts.URL,
		Token:   "test-token",
	}
	config.GlobalMaxWorkers = 4
	config.GlobalLogger = slog.New(slog.NewTextHandler(io.Discard, nil))

	client, err := NewClient(context.Background())
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	return client
}

// metadataHandler is the standard core API response with numeric project/repo ids.
func metadataHandler(w http.ResponseWriter, r *http.Request) {
	if strings.HasPrefix(r.URL.Path, "/api/latest/projects/") {
		// verify Bearer
		if r.Header.Get("Authorization") != "Bearer test-token" {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`unauthorized`))
			return
		}
		_, _ = w.Write([]byte(`{"id":42,"slug":"r00","project":{"id":7,"key":"PK"}}`))
		return
	}
	w.WriteHeader(http.StatusNotFound)
}

func TestGetMergeChecks(t *testing.T) {
	var mu sync.Mutex
	var gotQuery map[string]string
	client := scriptrunnerTestSetup(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasPrefix(r.URL.Path, "/api/latest/projects/"):
			metadataHandler(w, r)
		case r.URL.Path == "/scriptrunner-bitbucket/latest/mergechecks" && r.Method == http.MethodGet:
			q := r.URL.Query()
			mu.Lock()
			gotQuery = map[string]string{
				"repo":         q.Get("repo"),
				"repositoryId": q.Get("repositoryId"),
				"project":      q.Get("project"),
				"projectId":    q.Get("projectId"),
			}
			mu.Unlock()
			_, _ = w.Write([]byte(`[
				{"id":1,"name":"Prevent merge of pull requests behind target branch","disabled":false,"OUT_OF_DATE_STRATEGY":"REBASE","canned-script":"com.onresolve.scriptrunner.canned.bitbucket.mergechecks.BlockOutOfDatePullRequestsMergeCheck"},
				{"id":2,"name":"Conditional merge check","disabled":true,"canned-script":"com.onresolve.scriptrunner.canned.bitbucket.mergechecks.ConditionalMergeCheck"}
			]`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	})

	repos := []models.ExtendedRepository{{ProjectKey: "PK", RepositorySlug: "r00"}}
	out, err := client.GetMergeChecks(repos)
	if err != nil {
		t.Fatalf("GetMergeChecks: %v", err)
	}
	if len(out) != 1 || out[0].MergeChecks == nil {
		t.Fatalf("expected merge checks in result, got %+v", out)
	}
	if len(*out[0].MergeChecks) != 2 {
		t.Fatalf("expected 2 merge checks, got %d", len(*out[0].MergeChecks))
	}
	name, _ := (*out[0].MergeChecks)[0]["name"].(string)
	if name != "Prevent merge of pull requests behind target branch" {
		t.Fatalf("unexpected name: %q", name)
	}

	mu.Lock()
	defer mu.Unlock()
	if gotQuery["repo"] != "r00" || gotQuery["repositoryId"] != "42" || gotQuery["project"] != "PK" || gotQuery["projectId"] != "7" {
		t.Fatalf("query mismatch: %+v", gotQuery)
	}
}

func TestUpsertMergeChecks(t *testing.T) {
	var mu sync.Mutex
	var postedPath string
	var postedBody map[string]any
	client := scriptrunnerTestSetup(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasPrefix(r.URL.Path, "/api/latest/projects/"):
			metadataHandler(w, r)
		case strings.HasPrefix(r.URL.Path, "/scriptrunner-bitbucket/latest/mergechecks/") && r.Method == http.MethodPost:
			body, _ := io.ReadAll(r.Body)
			mu.Lock()
			postedPath = r.URL.Path
			_ = json.Unmarshal(body, &postedBody)
			mu.Unlock()
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(body)
		case strings.HasPrefix(r.URL.Path, "/api/latest/projects/"):
			metadataHandler(w, r)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	})

	check := models.ScriptRunnerMergeCheck{
		"id":            float64(5),
		"name":          "Conditional merge check",
		"disabled":      false,
		"canned-script": "com.onresolve.scriptrunner.canned.bitbucket.mergechecks.ConditionalMergeCheck",
	}
	repos := []models.ExtendedRepository{{ProjectKey: "PK", RepositorySlug: "r00", MergeChecks: &[]models.ScriptRunnerMergeCheck{check}}}

	out, err := client.UpsertMergeChecks(repos)
	if err != nil {
		t.Fatalf("UpsertMergeChecks: %v", err)
	}
	if len(out) != 1 || out[0].MergeChecks == nil || len(*out[0].MergeChecks) != 1 {
		t.Fatalf("expected upserted merge check in result, got %+v", out)
	}

	mu.Lock()
	defer mu.Unlock()
	if !strings.HasSuffix(postedPath, "/mergechecks/com.onresolve.scriptrunner.canned.bitbucket.mergechecks.ConditionalMergeCheck") {
		t.Fatalf("POST path should contain canned-script, got: %s", postedPath)
	}
	if postedBody["name"] != "Conditional merge check" {
		t.Fatalf("unexpected posted body: %+v", postedBody)
	}
}

func TestUpsertMergeChecks_MissingCanned(t *testing.T) {
	client := scriptrunnerTestSetup(t, func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/latest/projects/") {
			metadataHandler(w, r)
			return
		}
		w.WriteHeader(http.StatusOK)
	})

	check := models.ScriptRunnerMergeCheck{"name": "no canned"}
	repos := []models.ExtendedRepository{{ProjectKey: "PK", RepositorySlug: "r00", MergeChecks: &[]models.ScriptRunnerMergeCheck{check}}}

	_, err := client.UpsertMergeChecks(repos)
	if err == nil {
		t.Fatal("expected error for missing canned-script")
	}
	if !strings.Contains(err.Error(), "canned-script") {
		t.Fatalf("error should mention canned-script, got: %v", err)
	}
}

func TestDeleteMergeChecks(t *testing.T) {
	var mu sync.Mutex
	var deletePath string
	var deleteQuery map[string]string
	client := scriptrunnerTestSetup(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete && strings.HasPrefix(r.URL.Path, "/scriptrunner-bitbucket/latest/mergechecks/") {
			q := r.URL.Query()
			mu.Lock()
			deletePath = r.URL.Path
			deleteQuery = map[string]string{
				"repo":         q.Get("repo"),
				"repositoryId": q.Get("repositoryId"),
				"project":      q.Get("project"),
			}
			mu.Unlock()
			w.WriteHeader(http.StatusNoContent)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	})

	check := models.ScriptRunnerMergeCheck{
		"id":               float64(9),
		"FIELD_PER_REPO_ID": float64(42),
	}
	repos := []models.ExtendedRepository{{ProjectKey: "PK", RepositorySlug: "r00", MergeChecks: &[]models.ScriptRunnerMergeCheck{check}}}

	if err := client.DeleteMergeChecks(repos); err != nil {
		t.Fatalf("DeleteMergeChecks: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()
	if !strings.HasSuffix(deletePath, "/mergechecks/9") {
		t.Fatalf("DELETE path should contain id 9, got: %s", deletePath)
	}
	if deleteQuery["repo"] != "r00" || deleteQuery["repositoryId"] != "42" || deleteQuery["project"] != "PK" {
		t.Fatalf("delete query mismatch: %+v", deleteQuery)
	}
}

func TestDeleteMergeChecks_404IsNotError(t *testing.T) {
	client := scriptrunnerTestSetup(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.WriteHeader(http.StatusOK)
	})

	check := models.ScriptRunnerMergeCheck{"id": float64(11)}
	repos := []models.ExtendedRepository{{ProjectKey: "PK", RepositorySlug: "r00", MergeChecks: &[]models.ScriptRunnerMergeCheck{check}}}

	if err := client.DeleteMergeChecks(repos); err != nil {
		t.Fatalf("404 delete should not be an error, got: %v", err)
	}
}

func TestGetDefaultTasks(t *testing.T) {
	client := scriptrunnerTestSetup(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/default-tasks/latest/projects/PK/repos/r00/tasks") {
			_, _ = w.Write([]byte(`{"size":1,"limit":25,"isLastPage":true,"start":0,"values":[{"id":77,"description":"You have automatic versioning enabled","sourceMatcher":{"id":"ANY_REF_MATCHER_ID"},"targetMatcher":{"id":"refs/heads/trunk"}}]}`))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	})

	repos := []models.ExtendedRepository{{ProjectKey: "PK", RepositorySlug: "r00"}}
	out, err := client.GetDefaultTasks(repos)
	if err != nil {
		t.Fatalf("GetDefaultTasks: %v", err)
	}
	if len(out) != 1 || out[0].DefaultTasks == nil || len(*out[0].DefaultTasks) != 1 {
		t.Fatalf("expected default tasks in result, got %+v", out)
	}
	desc, _ := (*out[0].DefaultTasks)[0]["description"].(string)
	if !strings.Contains(desc, "versioning") {
		t.Fatalf("unexpected description: %q", desc)
	}
}

func TestCreateDefaultTasks(t *testing.T) {
	var mu sync.Mutex
	var postedBody map[string]any
	client := scriptrunnerTestSetup(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && strings.HasPrefix(r.URL.Path, "/default-tasks/latest/projects/PK/repos/r00/tasks") {
			body, _ := io.ReadAll(r.Body)
			mu.Lock()
			_ = json.Unmarshal(body, &postedBody)
			mu.Unlock()
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(body)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	})

	task := models.DefaultTask{
		"sourceMatcher": map[string]any{"id": "ANY_REF_MATCHER_ID"},
		"targetMatcher": map[string]any{"id": "refs/heads/trunk"},
		"description":   "You have automatic versioning enabled",
	}
	repos := []models.ExtendedRepository{{ProjectKey: "PK", RepositorySlug: "r00", DefaultTasks: &[]models.DefaultTask{task}}}

	out, err := client.CreateDefaultTasks(repos)
	if err != nil {
		t.Fatalf("CreateDefaultTasks: %v", err)
	}
	if len(out) != 1 || out[0].DefaultTasks == nil || len(*out[0].DefaultTasks) != 1 {
		t.Fatalf("expected created default task in result, got %+v", out)
	}
	mu.Lock()
	defer mu.Unlock()
	if postedBody["description"] != task["description"] {
		t.Fatalf("unexpected posted body: %+v", postedBody)
	}
}

func TestAreMergeChecksEqual_SubsetSemantics(t *testing.T) {
	current := models.ScriptRunnerMergeCheck{
		"id":       float64(3),
		"name":     "Conditional merge check",
		"disabled": true,
		"extra":    "ScriptRunner computed field",
	}
	target := models.ScriptRunnerMergeCheck{
		"name":     "Conditional merge check",
		"disabled": true,
	}
	if !AreMergeChecksEqual(current, target) {
		t.Fatal("target subset of current must be equal (id ignored)")
	}

	target["disabled"] = false
	if AreMergeChecksEqual(current, target) {
		t.Fatal("different target key value must not be equal")
	}

	missing := models.ScriptRunnerMergeCheck{"name": "Other name"}
	if AreMergeChecksEqual(current, missing) {
		t.Fatal("target key missing in current must not be equal")
	}
}

func TestMergeUpdateMergeChecks(t *testing.T) {
	current := models.ScriptRunnerMergeCheck{
		"id":       float64(3),
		"name":     "Conditional merge check",
		"disabled": true,
		"keepMe":   "current-only field",
	}
	target := models.ScriptRunnerMergeCheck{
		"name":     "Conditional merge check",
		"disabled": false,
		"newField": "new",
	}
	merged := MergeUpdateMergeChecks(current, target)

	if merged["disabled"] != false {
		t.Fatal("target keys must override current")
	}
	if merged["keepMe"] != "current-only field" {
		t.Fatal("current-only keys must be preserved")
	}
	if _, ok := merged["newField"]; !ok {
		t.Fatal("new target keys must be present")
	}
	id, ok := merged["id"].(float64)
	if !ok || id != 3 {
		t.Fatalf("id must be preserved from current, got: %v", merged["id"])
	}
}

func TestGetMergeChecks_MetadataFailure(t *testing.T) {
	client := scriptrunnerTestSetup(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	})

	repos := []models.ExtendedRepository{{ProjectKey: "PK", RepositorySlug: "r00"}}
	_, err := client.GetMergeChecks(repos)
	if err == nil {
		t.Fatal("expected error when repo metadata is unavailable")
	}
}
