package repo

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

func TestRepoCommandTree(t *testing.T) {
	cmd := NewRepoCmd()
	if cmd.Use != "repo" {
		t.Fatalf("Use = %q", cmd.Use)
	}
	want := []string{
		"get", "create", "update", "delete", "fork",
		"webhook", "branch-permission", "required-build",
		"reviewer-group", "workzone", "permission",
	}
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

func TestRepoCommandFlags(t *testing.T) {
	tests := []struct {
		name  string
		cmd   *cobra.Command
		flags []string
	}{
		{"get", NewGetCmd(), []string{"projectKey", "repositorySlug", "columns", "output", "manifest-file", "config-file", "show-details", "input"}},
		{"create", NewCreateCmd(), []string{"projectKey", "name", "repositorySlug", "desc", "default-branch", "input", "output"}},
		{"update", NewUpdateCmd(), []string{"repositorySlug", "name", "desc", "default-branch", "input", "newProjectKey", "output"}},
		{"delete", NewDeleteCmd(), []string{"repositorySlug", "input"}},
		{"fork", NewForkCmd(), []string{"repositorySlug", "newProjectKey", "newName", "newDefaultBranch", "newDescription", "input", "output"}},
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

func TestRepoValidation(t *testing.T) {
	withTestConfig(t)

	emptyRepos := writeTempFile(t, "empty.yaml", "repositories: []\n")
	missingFile := filepath.Join(t.TempDir(), "missing.yaml")

	tests := []struct {
		name    string
		cmd     *cobra.Command
		args    []string
		wantErr string
	}{
		{"create without args", NewCreateCmd(), nil, "either --input or (--projectKey and --name) must be specified"},
		{"create key without name", NewCreateCmd(), []string{"--projectKey", "DEMO"}, "either --input or (--projectKey and --name) must be specified"},
		{"create input and flags", NewCreateCmd(), []string{"--input", emptyRepos, "--projectKey", "DEMO"}, "either --input or (--projectKey and --name) must be specified"},
		{"create empty input file", NewCreateCmd(), []string{"--input", emptyRepos}, "no repositories found"},
		{"create missing input file", NewCreateCmd(), []string{"--input", missingFile}, "failed to parse file"},
		{"update without args", NewUpdateCmd(), nil, "either --input or --repositorySlug must be specified, but not both"},
		{"update input and slug", NewUpdateCmd(), []string{"--input", emptyRepos, "--repositorySlug", "DEMO/app"}, "either --input or --repositorySlug must be specified, but not both"},
		{"update invalid slug", NewUpdateCmd(), []string{"--repositorySlug", "DEMO"}, "invalid repositorySlug format"},
		{"update missing input file", NewUpdateCmd(), []string{"--input", missingFile}, "failed to read file"},
		{"delete without args", NewDeleteCmd(), nil, "either --repositorySlug or --input must be specified, but not both"},
		{"delete input and slug", NewDeleteCmd(), []string{"--input", emptyRepos, "--repositorySlug", "DEMO/app"}, "either --repositorySlug or --input must be specified, but not both"},
		{"delete invalid slug", NewDeleteCmd(), []string{"--repositorySlug", "DEMO"}, "invalid repository identifier format"},
		{"delete empty input file", NewDeleteCmd(), []string{"--input", emptyRepos}, "no repositories found"},
		{"fork without args", NewForkCmd(), nil, "either --input, or --repositorySlug must be specified, but not both"},
		{"fork input and slug", NewForkCmd(), []string{"--input", emptyRepos, "--repositorySlug", "DEMO/app"}, "either --input, or --repositorySlug must be specified, but not both"},
		{"fork invalid slug", NewForkCmd(), []string{"--repositorySlug", "DEMO"}, "invalid repository identifier format"},
		{"get without args", NewGetCmd(), nil, "please specify exactly one of --projectKey, --repositorySlug or --input"},
		{"get two options", NewGetCmd(), []string{"--projectKey", "DEMO", "--repositorySlug", "DEMO/app"}, "please specify exactly one of --projectKey, --repositorySlug or --input"},
		{"get empty show-details", NewGetCmd(), []string{"--projectKey", "DEMO", "--show-details="}, "--show-details cannot be empty"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := runCmd(t, tt.cmd, tt.args...)
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("got error %v, want containing %q", err, tt.wantErr)
			}
		})
	}
}
