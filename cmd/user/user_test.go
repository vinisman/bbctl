package user

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

func TestUserCommandTree(t *testing.T) {
	cmd := UserCmd()
	if cmd.Use != "user" {
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

func TestUserCommandFlags(t *testing.T) {
	tests := []struct {
		name  string
		cmd   *cobra.Command
		flags []string
	}{
		{"get", GetCmd(), []string{"name", "all", "output", "input"}},
		{"create", CreateCmd(), []string{"name", "displayName", "email", "user-password", "input", "output"}},
		{"update", UpdateCmd(), []string{"name", "displayName", "email", "input", "output"}},
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

func TestUserValidation(t *testing.T) {
	withTestConfig(t)

	emptyUsers := writeTempFile(t, "empty.yaml", "users: []\n")
	userWithoutName := writeTempFile(t, "no-name.yaml", "users:\n  - displayName: Demo\n")
	userWithoutDisplayName := writeTempFile(t, "no-display.yaml", "users:\n  - name: demo\n")
	userWithoutEmail := writeTempFile(t, "no-email.yaml", "users:\n  - name: demo\n    displayName: Demo\n")
	validUsers := writeTempFile(t, "valid.yaml", "users:\n  - name: demo\n    displayName: Demo\n    emailAddress: demo@example.com\n")
	userNameOnly := writeTempFile(t, "name-only.yaml", "users:\n  - name: demo\n")

	tests := []struct {
		name    string
		cmd     *cobra.Command
		args    []string
		wantErr string
	}{
		{"create without args", CreateCmd(), nil, "either --name and --displayName or --input must be specified"},
		{"create name and input", CreateCmd(), []string{"--name", "demo", "--input", emptyUsers}, "either --name and --displayName or --input must be specified"},
		{"create without display name", CreateCmd(), []string{"--name", "demo"}, "--displayName is required"},
		{"create without email", CreateCmd(), []string{"--name", "demo", "--displayName", "Demo"}, "--email is required"},
		{"create without password", CreateCmd(), []string{"--name", "demo", "--displayName", "Demo", "--email", "demo@example.com"}, "--user-password is required"},
		{"create empty input file", CreateCmd(), []string{"--input", emptyUsers}, "no users found in file"},
		{"create input without display name", CreateCmd(), []string{"--input", userWithoutDisplayName}, "missing required field 'displayName'"},
		{"create input without email", CreateCmd(), []string{"--input", userWithoutEmail}, "missing required field 'emailAddress'"},
		{"create input without password", CreateCmd(), []string{"--input", validUsers}, "--user-password is required when creating users from file"},
		{"update without args", UpdateCmd(), nil, "either --name or --input must be specified, but not both"},
		{"update name and input", UpdateCmd(), []string{"--name", "demo", "--input", emptyUsers}, "either --name or --input must be specified, but not both"},
		{"update without changes", UpdateCmd(), []string{"--name", "demo"}, "at least one of --displayName or --email"},
		{"update empty input file", UpdateCmd(), []string{"--input", emptyUsers}, "no users found in file"},
		{"update input without name", UpdateCmd(), []string{"--input", userWithoutName}, "missing required field 'name'"},
		{"update input without changes", UpdateCmd(), []string{"--input", userNameOnly}, "must have at least one of 'displayName' or 'emailAddress'"},
		{"get without args", GetCmd(), nil, "please specify exactly one of -n/--name, --all, or --input"},
		{"get name and all", GetCmd(), []string{"--name", "demo", "--all"}, "please specify exactly one of -n/--name, --all, or --input"},
		{"get empty input file", GetCmd(), []string{"--input", emptyUsers}, "no users found in file"},
		{"get input without names", GetCmd(), []string{"--input", userWithoutName}, "no usernames found in file"},
		{"delete without args", DeleteCmd(), nil, "either --name or --input must be specified, but not both"},
		{"delete name and input", DeleteCmd(), []string{"--name", "demo", "--input", emptyUsers}, "either --name or --input must be specified, but not both"},
		{"delete empty input file", DeleteCmd(), []string{"--input", emptyUsers}, "no users found in file"},
		{"delete input without names", DeleteCmd(), []string{"--input", userWithoutName}, "no usernames found in file"},
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
