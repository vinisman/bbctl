package group

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

func TestGroupCommandTree(t *testing.T) {
	cmd := GroupCmd()
	if cmd.Use != "group" {
		t.Fatalf("Use = %q", cmd.Use)
	}
	want := []string{"get", "create", "delete"}
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

func TestGroupCommandFlags(t *testing.T) {
	tests := []struct {
		name  string
		cmd   *cobra.Command
		flags []string
	}{
		{"get", GetCmd(), []string{"name", "all", "output", "input"}},
		{"create", CreateCmd(), []string{"name", "input", "output"}},
		{"delete", DeleteCmd(), []string{"name", "input", "output"}},
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

func TestGroupValidation(t *testing.T) {
	withTestConfig(t)

	emptyGroups := writeTempFile(t, "empty.yaml", "groups: []\n")
	groupWithoutName := writeTempFile(t, "no-name.yaml", "groups:\n  - {}\n")

	tests := []struct {
		name    string
		cmd     *cobra.Command
		args    []string
		wantErr string
	}{
		{"create without args", CreateCmd(), nil, "either --name or --input must be specified, but not both"},
		{"create name and input", CreateCmd(), []string{"--name", "devs", "--input", emptyGroups}, "either --name or --input must be specified, but not both"},
		{"create empty input file", CreateCmd(), []string{"--input", emptyGroups}, "no groups found in file"},
		{"create group without name", CreateCmd(), []string{"--input", groupWithoutName}, "missing required field 'name'"},
		{"get without args", GetCmd(), nil, "please specify exactly one of -n/--name, --all, or --input"},
		{"get name and all", GetCmd(), []string{"--name", "devs", "--all"}, "please specify exactly one of -n/--name, --all, or --input"},
		{"get empty input file", GetCmd(), []string{"--input", emptyGroups}, "no groups found in file"},
		{"get input without names", GetCmd(), []string{"--input", groupWithoutName}, "no group names found in file"},
		{"delete without args", DeleteCmd(), nil, "either --name or --input must be specified, but not both"},
		{"delete name and input", DeleteCmd(), []string{"--name", "devs", "--input", emptyGroups}, "either --name or --input must be specified, but not both"},
		{"delete empty input file", DeleteCmd(), []string{"--input", emptyGroups}, "no groups found in file"},
		{"delete input without names", DeleteCmd(), []string{"--input", groupWithoutName}, "no group names found in file"},
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
