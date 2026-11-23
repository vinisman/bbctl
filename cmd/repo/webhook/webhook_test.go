package webhook

import (
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/vinisman/bbctl/internal/config"
	"github.com/vinisman/bbctl/internal/models"
	openapi "github.com/vinisman/bitbucket-sdk-go/openapi"
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

func int32Ptr(v int32) *int32 { return &v }

func wh(id *int32, name string) openapi.RestWebhook {
	return openapi.RestWebhook{Id: id, Name: &name, Url: strPtr("https://ci.example.com/" + name)}
}

func whRepo(project, slug string, items ...openapi.RestWebhook) models.ExtendedRepository {
	r := models.ExtendedRepository{ProjectKey: project, RepositorySlug: slug}
	if items != nil {
		r.Webhooks = &items
	}
	return r
}

func writeTempFile(t *testing.T, name, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestRepoWebHookCommandTree(t *testing.T) {
	cmd := RepoWebHookCmd()
	if cmd.Use != "webhook" {
		t.Fatalf("Use = %q", cmd.Use)
	}
	want := []string{"get", "create", "update", "delete", "diff"}
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

func TestWebhookCommandFlags(t *testing.T) {
	tests := []struct {
		name  string
		cmd   *cobra.Command
		flags []string
	}{
		{"get", GetWebHookCmd(), []string{"repositorySlug", "output", "input"}},
		{"create", CreateWebHookCmd(), []string{"input", "output"}},
		{"update", UpdateWebHookCmd(), []string{"input", "output"}},
		{"delete", DeleteWebHookCmd(), []string{"input", "projectKey", "repositorySlug", "ids"}},
		{"diff", DiffWebHookCmd(), []string{"source", "target", "output", "apply", "force-update", "apply-result-out", "apply-rollback-out", "rollback", "quiet"}},
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

func TestWebhookValidation(t *testing.T) {
	withTestConfig(t)

	emptyFile := writeTempFile(t, "empty.yaml", "repositories: []\n")
	repoWithoutItems := writeTempFile(t, "repo.yaml", "repositories:\n  - projectKey: DEMO\n    repositorySlug: app\n")

	tests := []struct {
		name    string
		cmd     *cobra.Command
		args    []string
		wantErr string
	}{
		{"create without input", CreateWebHookCmd(), nil, "--input is required"},
		{"update without input", UpdateWebHookCmd(), nil, "--input is required"},
		{"create empty file", CreateWebHookCmd(), []string{"--input", emptyFile}, ""},
		{"update empty file", UpdateWebHookCmd(), []string{"--input", emptyFile}, ""},
		{"create without webhooks", CreateWebHookCmd(), []string{"--input", repoWithoutItems}, "no webhooks defined"},
		{"delete without args", DeleteWebHookCmd(), nil, "either --input or all of --projectKey, --repositorySlug, and --ids"},
		{"delete input and flags", DeleteWebHookCmd(), []string{"--input", emptyFile, "--projectKey", "DEMO"}, "--input cannot be used together"},
		{"delete invalid id", DeleteWebHookCmd(), []string{"--projectKey", "DEMO", "--repositorySlug", "app", "--ids", "abc"}, "invalid webhook id"},
		{"delete id out of range", DeleteWebHookCmd(), []string{"--projectKey", "DEMO", "--repositorySlug", "app", "--ids", "3000000000"}, "out of int32 range"},
		{"get without args", GetWebHookCmd(), nil, "please specify --repositorySlug"},
		{"get invalid identifier", GetWebHookCmd(), []string{"--repositorySlug", "DEMO"}, "invalid repository identifier format"},
		{"diff missing files", DiffWebHookCmd(), nil, "both --source and --target are required"},
		{"diff apply-result-out without apply", DiffWebHookCmd(), []string{"--source", emptyFile, "--target", emptyFile, "--apply-result-out", "out.json"}, "--apply-result-out can only be used together with --apply"},
		{"diff apply-rollback-out without apply", DiffWebHookCmd(), []string{"--source", emptyFile, "--target", emptyFile, "--apply-rollback-out", "out.json"}, "--apply-rollback-out can only be used together with --apply"},
		{"diff rollback with apply", DiffWebHookCmd(), []string{"--rollback", "plan.json", "--apply"}, "--rollback cannot be combined with --apply"},
		{"diff invalid output", DiffWebHookCmd(), []string{"--source", emptyFile, "--target", emptyFile, "-o", "xml"}, "invalid output format"},
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

func TestGenerateWebhookDiff(t *testing.T) {
	src := []models.ExtendedRepository{
		whRepo("DEMO", "app", wh(int32Ptr(1), "old")),
		whRepo("OPS", "old", wh(int32Ptr(5), "gone")),
	}
	tgt := []models.ExtendedRepository{
		whRepo("DEMO", "app",
			wh(int32Ptr(1), "new"),
			wh(int32Ptr(2), "added"),
			wh(nil, "no-id"),
		),
		whRepo("DEMO", "new", wh(int32Ptr(9), "brand-new")),
	}

	diff, err := generateWebhookDiff(src, tgt)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(diff.Create) != 2 {
		t.Fatalf("create repos = %d, want 2", len(diff.Create))
	}
	if len(*diff.Create[0].Webhooks) != 2 {
		t.Fatalf("create items = %d, want 2", len(*diff.Create[0].Webhooks))
	}
	if len(diff.Update) != 1 {
		t.Fatalf("update repos = %d, want 1", len(diff.Update))
	}
	if len(diff.Delete) != 1 || diff.Delete[0].ProjectKey != "OPS" {
		t.Fatalf("unexpected delete: %+v", diff.Delete)
	}
}

func TestGenerateWebhookDiff_NoItems(t *testing.T) {
	src := []models.ExtendedRepository{whRepo("DEMO", "app")}
	tgt := []models.ExtendedRepository{whRepo("DEMO", "app")}

	diff, err := generateWebhookDiff(src, tgt)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(diff.Create) != 0 || len(diff.Update) != 0 || len(diff.Delete) != 0 {
		t.Fatalf("expected empty diff, got %+v", diff)
	}
}

func strPtr(s string) *string { return &s }
