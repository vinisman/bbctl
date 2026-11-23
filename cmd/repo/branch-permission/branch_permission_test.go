package branchpermission

import (
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/vinisman/bbctl/internal/config"
)

func withTestConfig(t *testing.T) {
	t.Helper()
	prevCfg, prevLogger := config.GlobalCfg, config.GlobalLogger
	config.GlobalCfg = &config.Config{BaseURL: "http://127.0.0.1:1", Token: "test-token"}
	config.GlobalLogger = slog.New(slog.NewTextHandler(io.Discard, nil))
	t.Cleanup(func() {
		config.GlobalCfg, config.GlobalLogger = prevCfg, prevLogger
	})
}

func runCmd(t *testing.T, cmd *cobra.Command, args ...string) error {
	t.Helper()
	cmd.SetArgs(args)
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	return cmd.Execute()
}

func writeTempFile(t *testing.T, name, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestBranchPermissionCommandTree(t *testing.T) {
	cmd := RepoBranchPermissionCmd()
	if cmd.Use != "branch-permission" {
		t.Fatalf("Use = %q", cmd.Use)
	}
	want := []string{"get", "create", "update", "delete"}
	got := map[string]bool{}
	for _, sub := range cmd.Commands() {
		got[sub.Name()] = true
	}
	for _, name := range want {
		if !got[name] {
			t.Fatalf("missing subcommand %q", name)
		}
	}
}

func TestBranchPermissionCommandFlags(t *testing.T) {
	tests := []struct {
		name  string
		cmd   *cobra.Command
		flags []string
	}{
		{"get", GetBranchPermissionCmd(), []string{"repositorySlug", "output", "input"}},
		{"create", CreateBranchPermissionCmd(), []string{"input", "output"}},
		{"update", UpdateBranchPermissionCmd(), []string{"input", "output"}},
		{"delete", DeleteBranchPermissionCmd(), []string{"input", "repositorySlug", "ids"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for _, flag := range tt.flags {
				if tt.cmd.Flags().Lookup(flag) == nil {
					t.Fatalf("missing flag %q", flag)
				}
			}
		})
	}
}

func TestBranchPermissionValidation(t *testing.T) {
	withTestConfig(t)

	emptyRepos := writeTempFile(t, "empty.yaml", "repositories: []\n")
	repoWithoutPermissions := writeTempFile(t, "no-permissions.yaml", "repositories:\n  - projectKey: DEMO\n    repositorySlug: app\n")

	tests := []struct {
		name    string
		cmd     *cobra.Command
		args    []string
		wantErr string
	}{
		{"create without input", CreateBranchPermissionCmd(), nil, "--input is required"},
		{"update without input", UpdateBranchPermissionCmd(), nil, "--input is required"},
		{"create empty file", CreateBranchPermissionCmd(), []string{"--input", emptyRepos}, ""},
		{"update empty file", UpdateBranchPermissionCmd(), []string{"--input", emptyRepos}, ""},
		{"create without permissions", CreateBranchPermissionCmd(), []string{"--input", repoWithoutPermissions}, "no branch permissions defined"},
		{"update without permissions", UpdateBranchPermissionCmd(), []string{"--input", repoWithoutPermissions}, "no branch permissions defined"},
		{"delete without args", DeleteBranchPermissionCmd(), nil, "please specify --repositorySlug and --ids, or use --input"},
		{"delete invalid identifier", DeleteBranchPermissionCmd(), []string{"--repositorySlug", "DEMO", "--ids", "1"}, "invalid repository identifier format"},
		{"delete empty input file", DeleteBranchPermissionCmd(), []string{"--input", emptyRepos}, "no branch permissions defined for deletion"},
		{"get without args", GetBranchPermissionCmd(), nil, "please specify --repositorySlug"},
		{"get invalid identifier", GetBranchPermissionCmd(), []string{"--repositorySlug", "DEMO"}, "invalid repository identifier format"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := runCmd(t, tt.cmd, tt.args...)
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("got error %v, want containing %q", err, tt.wantErr)
			}
		})
	}
}
