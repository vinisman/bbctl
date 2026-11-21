package permission

import (
	"strings"
	"testing"

	"github.com/vinisman/bbctl/internal/models"
)

func TestParseRepositories(t *testing.T) {
	repos, err := parseRepositories("DEMO/app-web, OPS/monitoring ,,")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(repos) != 2 {
		t.Fatalf("got %d repos, want 2: %+v", len(repos), repos)
	}
	if repos[0].ProjectKey != "DEMO" || repos[0].RepositorySlug != "app-web" {
		t.Fatalf("unexpected first repo: %+v", repos[0])
	}
	if repos[1].ProjectKey != "OPS" || repos[1].RepositorySlug != "monitoring" {
		t.Fatalf("unexpected second repo: %+v", repos[1])
	}
}

func TestParseRepositories_Invalid(t *testing.T) {
	for _, in := range []string{"DEMO", "DEMO/", "/app-web"} {
		t.Run(in, func(t *testing.T) {
			if _, err := parseRepositories(in); err == nil {
				t.Fatalf("expected error for %q", in)
			}
		})
	}
}

func TestBuildRepositories(t *testing.T) {
	repos, err := buildRepositories("DEMO/app-web", "jdoe,asmith", "devs", "repo_read")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(repos) != 1 {
		t.Fatalf("got %d repos, want 1", len(repos))
	}
	if len(repos[0].Users) != 2 || len(repos[0].Groups) != 1 {
		t.Fatalf("unexpected subjects: %+v", repos[0])
	}
}

func TestValidateRepositories_Grant(t *testing.T) {
	tests := []struct {
		name      string
		repos     []models.RepositoryPermissions
		wantErr   string
		wantLevel string
	}{
		{
			name: "normalizes permission",
			repos: []models.RepositoryPermissions{{
				ProjectKey: "DEMO", RepositorySlug: "app-web",
				Users: []models.PermissionEntry{{Name: "jdoe", Permission: "repo_admin"}},
			}},
			wantLevel: "REPO_ADMIN",
		},
		{
			name: "missing slug",
			repos: []models.RepositoryPermissions{{
				ProjectKey: "DEMO",
				Users:      []models.PermissionEntry{{Name: "jdoe", Permission: "REPO_READ"}},
			}},
			wantErr: "projectKey and repositorySlug are required",
		},
		{
			name: "no subjects",
			repos: []models.RepositoryPermissions{{
				ProjectKey: "DEMO", RepositorySlug: "app-web",
			}},
			wantErr: "no users or groups specified",
		},
		{
			name: "wrong permission scope",
			repos: []models.RepositoryPermissions{{
				ProjectKey: "DEMO", RepositorySlug: "app-web",
				Users: []models.PermissionEntry{{Name: "jdoe", Permission: "PROJECT_READ"}},
			}},
			wantErr: "invalid permission",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateRepositories(tt.repos, true)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("got error %v, want containing %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tt.wantLevel != "" && tt.repos[0].Users[0].Permission != tt.wantLevel {
				t.Fatalf("permission not normalized: %+v", tt.repos[0].Users[0])
			}
		})
	}
}

func TestValidateRepositories_Revoke(t *testing.T) {
	repos := []models.RepositoryPermissions{{
		ProjectKey:     "DEMO",
		RepositorySlug: "app-web",
		Users:          []models.PermissionEntry{{Name: "jdoe"}},
	}}
	if err := validateRepositories(repos, false); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRepositoryPermissionRows(t *testing.T) {
	rows := repositoryPermissionRows([]models.RepositoryPermissions{{
		ProjectKey:     "DEMO",
		RepositorySlug: "app-web",
		Users:          []models.PermissionEntry{{Name: "jdoe", Permission: "REPO_READ"}},
		Groups:         []models.PermissionEntry{{Name: "devs", Permission: "REPO_WRITE"}},
	}})
	if len(rows) != 2 {
		t.Fatalf("got %d rows, want 2", len(rows))
	}
	for _, r := range rows {
		if r.ProjectKey != "DEMO" || r.RepositorySlug != "app-web" {
			t.Fatalf("unexpected scope: %+v", r)
		}
	}
	if rows[0].Type != "user" || rows[1].Type != "group" {
		t.Fatalf("unexpected row types: %+v", rows)
	}
}
