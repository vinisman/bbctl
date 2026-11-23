package project

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

func TestProjectCommandTree(t *testing.T) {
	cmd := NewProjectCmd()
	if cmd.Use != "project" {
		t.Fatalf("Use = %q", cmd.Use)
	}
	want := []string{"get", "create", "update", "delete", "permission"}
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

func TestProjectCommandFlags(t *testing.T) {
	tests := []struct {
		name  string
		cmd   *cobra.Command
		flags []string
	}{
		{"get", NewGetCmd(), []string{"key", "all", "output", "input"}},
		{"create", NewCreateCmd(), []string{"key", "name", "description", "input", "output"}},
		{"update", NewUpdateCmd(), []string{"key", "name", "des", "input", "output"}},
		{"delete", NewDeleteCmd(), []string{"key", "input"}},
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

func TestProjectValidation(t *testing.T) {
	withTestConfig(t)

	emptyProjects := writeTempFile(t, "empty.yaml", "projects: []\n")
	projectWithoutKey := writeTempFile(t, "no-key.yaml", "projects:\n  - name: Demo\n")
	missingFile := filepath.Join(t.TempDir(), "missing.yaml")

	tests := []struct {
		name    string
		cmd     *cobra.Command
		args    []string
		wantErr string
	}{
		{"create without args", NewCreateCmd(), nil, "either --key and --name or --input must be specified"},
		{"create key and input", NewCreateCmd(), []string{"--key", "DEMO", "--input", emptyProjects}, "either --key and --name or --input must be specified"},
		{"create key without name", NewCreateCmd(), []string{"--key", "DEMO"}, "--name is required"},
		{"create missing input file", NewCreateCmd(), []string{"--input", missingFile}, "failed to parse file"},
		{"create empty input file", NewCreateCmd(), []string{"--input", emptyProjects}, "no projects found in file"},
		{"update without args", NewUpdateCmd(), nil, "either --key and --name or --input must be specified"},
		{"update key without name", NewUpdateCmd(), []string{"--key", "DEMO"}, "--name is required"},
		{"update empty input file", NewUpdateCmd(), []string{"--input", emptyProjects}, "no projects found in file"},
		{"get without args", NewGetCmd(), nil, "please specify exactly one of --key, --all, or --input"},
		{"get key and all", NewGetCmd(), []string{"--key", "DEMO", "--all"}, "please specify exactly one of --key, --all, or --input"},
		{"get input without keys", NewGetCmd(), []string{"--input", projectWithoutKey}, "no project keys found in file"},
		{"delete without args", NewDeleteCmd(), nil, "either --key or --input must be specified, but not both"},
		{"delete key and input", NewDeleteCmd(), []string{"--key", "DEMO", "--input", emptyProjects}, "either --key or --input must be specified, but not both"},
		{"delete empty input file", NewDeleteCmd(), []string{"--input", emptyProjects}, "no projects found in file"},
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
