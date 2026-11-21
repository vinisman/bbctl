package models

import "testing"

func TestNormalizePermission(t *testing.T) {
	tests := []struct {
		name    string
		level   string
		allowed []string
		want    string
		wantErr bool
	}{
		{"project read", "PROJECT_READ", ProjectPermissionLevels, "PROJECT_READ", false},
		{"lowercase normalized", "project_admin", ProjectPermissionLevels, "PROJECT_ADMIN", false},
		{"spaces trimmed", "  PROJECT_WRITE  ", ProjectPermissionLevels, "PROJECT_WRITE", false},
		{"repo write", "REPO_WRITE", RepositoryPermissionLevels, "REPO_WRITE", false},
		{"repo admin lowercase", "repo_admin", RepositoryPermissionLevels, "REPO_ADMIN", false},
		{"wrong scope", "PROJECT_READ", RepositoryPermissionLevels, "", true},
		{"unknown level", "PROJECT_OWNER", ProjectPermissionLevels, "", true},
		{"empty", "", ProjectPermissionLevels, "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := NormalizePermission(tt.level, tt.allowed)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error, got %q", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Fatalf("got %q, want %q", got, tt.want)
			}
		})
	}
}
