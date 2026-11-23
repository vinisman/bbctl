package reviewergroup

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

func TestReviewerGroupCommandTree(t *testing.T) {
	cmd := RepoReviewerGroupCmd()
	if cmd.Use != "reviewer-group" {
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

func TestReviewerGroupCommandFlags(t *testing.T) {
	tests := []struct {
		name  string
		cmd   *cobra.Command
		flags []string
	}{
		{"get", GetReviewerGroupCmd(), []string{"repositorySlug", "output", "input"}},
		{"create", CreateReviewerGroupCmd(), []string{"input", "output"}},
		{"update", UpdateReviewerGroupCmd(), []string{"input", "output"}},
		{"delete", DeleteReviewerGroupCmd(), []string{"input", "projectKey", "repositorySlug", "ids"}},
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

func TestReviewerGroupValidation(t *testing.T) {
	withTestConfig(t)

	emptyRepos := writeTempFile(t, "empty.yaml", "repositories: []\n")

	tests := []struct {
		name    string
		cmd     *cobra.Command
		args    []string
		wantErr string
	}{
		{"create without input", CreateReviewerGroupCmd(), nil, "input"},
		{"create empty file", CreateReviewerGroupCmd(), []string{"--input", emptyRepos}, ""},
		{"update without input", UpdateReviewerGroupCmd(), nil, "--input is required"},
		{"update empty file", UpdateReviewerGroupCmd(), []string{"--input", emptyRepos}, ""},
		{"delete without args", DeleteReviewerGroupCmd(), nil, "either --input or all of --projectKey, --repositorySlug, and --ids must be provided"},
		{"delete input and flags", DeleteReviewerGroupCmd(), []string{"--input", emptyRepos, "--projectKey", "DEMO"}, "--input cannot be used together"},
		{"delete invalid id", DeleteReviewerGroupCmd(), []string{"--projectKey", "DEMO", "--repositorySlug", "app", "--ids", "abc"}, "invalid reviewer group id"},
		{"delete empty file", DeleteReviewerGroupCmd(), []string{"--input", emptyRepos}, ""},
		{"get without args", GetReviewerGroupCmd(), nil, "please specify --repositorySlug"},
		{"get invalid identifier", GetReviewerGroupCmd(), []string{"--repositorySlug", "DEMO"}, "invalid repository identifier format"},
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
