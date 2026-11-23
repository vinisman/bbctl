package permission

import (
	"strings"
	"testing"

	"github.com/vinisman/bbctl/internal/models"
)

func TestParseProjectKeys(t *testing.T) {
	projects := parseProjectKeys(" DEMO , OPS,, ")
	if len(projects) != 2 {
		t.Fatalf("got %d projects, want 2: %+v", len(projects), projects)
	}
	if projects[0].ProjectKey != "DEMO" || projects[1].ProjectKey != "OPS" {
		t.Fatalf("unexpected keys: %+v", projects)
	}
}

func TestBuildProjects(t *testing.T) {
	projects := buildProjects("DEMO,OPS", "jdoe, asmith", "devs", "project_read")
	if len(projects) != 2 {
		t.Fatalf("got %d projects, want 2", len(projects))
	}
	for _, p := range projects {
		if len(p.Users) != 2 {
			t.Fatalf("project %s: got %d users, want 2", p.ProjectKey, len(p.Users))
		}
		if len(p.Groups) != 1 {
			t.Fatalf("project %s: got %d groups, want 1", p.ProjectKey, len(p.Groups))
		}
		if p.Users[0].Permission != "project_read" {
			t.Fatalf("unexpected permission: %+v", p.Users[0])
		}
	}
}

func TestValidateProjects_Grant(t *testing.T) {
	tests := []struct {
		name      string
		projects  []models.ProjectPermissions
		wantErr   string
		wantLevel string
	}{
		{
			name:      "normalizes permission",
			projects:  []models.ProjectPermissions{{ProjectKey: "DEMO", Users: []models.PermissionEntry{{Name: "jdoe", Permission: "project_write"}}}},
			wantLevel: "PROJECT_WRITE",
		},
		{
			name:     "missing project key",
			projects: []models.ProjectPermissions{{Users: []models.PermissionEntry{{Name: "jdoe", Permission: "PROJECT_READ"}}}},
			wantErr:  "projectKey is required",
		},
		{
			name:     "no subjects",
			projects: []models.ProjectPermissions{{ProjectKey: "DEMO"}},
			wantErr:  "no users or groups specified",
		},
		{
			name:     "invalid permission",
			projects: []models.ProjectPermissions{{ProjectKey: "DEMO", Groups: []models.PermissionEntry{{Name: "devs", Permission: "REPO_READ"}}}},
			wantErr:  "invalid permission",
		},
		{
			name:     "empty subject name",
			projects: []models.ProjectPermissions{{ProjectKey: "DEMO", Users: []models.PermissionEntry{{Permission: "PROJECT_READ"}}}},
			wantErr:  "name is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateProjects(tt.projects, true)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("got error %v, want containing %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tt.wantLevel != "" && tt.projects[0].Users[0].Permission != tt.wantLevel {
				t.Fatalf("permission not normalized: %+v", tt.projects[0].Users[0])
			}
		})
	}
}

func TestValidateProjects_Revoke(t *testing.T) {
	// Revoke does not require permission levels.
	projects := []models.ProjectPermissions{{
		ProjectKey: "DEMO",
		Users:      []models.PermissionEntry{{Name: "jdoe"}},
		Groups:     []models.PermissionEntry{{Name: "devs"}},
	}}
	if err := validateProjects(projects, false); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestProjectPermissionRows(t *testing.T) {
	rows := projectPermissionRows([]models.ProjectPermissions{{
		ProjectKey: "DEMO",
		Users:      []models.PermissionEntry{{Name: "jdoe", Permission: "PROJECT_READ"}},
		Groups:     []models.PermissionEntry{{Name: "devs", Permission: "PROJECT_ADMIN"}},
	}})
	if len(rows) != 2 {
		t.Fatalf("got %d rows, want 2", len(rows))
	}
	if rows[0].Type != "user" || rows[1].Type != "group" {
		t.Fatalf("unexpected row types: %+v", rows)
	}
	if rows[0].ProjectKey != "DEMO" || rows[1].Permission != "PROJECT_ADMIN" {
		t.Fatalf("unexpected rows: %+v", rows)
	}
}
