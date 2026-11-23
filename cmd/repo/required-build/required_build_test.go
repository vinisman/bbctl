package requiredbuild

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

func int64Ptr(v int64) *int64 { return &v }

func rb(id *int64, keys ...string) openapi.RestRequiredBuildCondition {
	return openapi.RestRequiredBuildCondition{Id: id, BuildParentKeys: keys}
}

func rbRepo(project, slug string, items ...openapi.RestRequiredBuildCondition) models.ExtendedRepository {
	r := models.ExtendedRepository{ProjectKey: project, RepositorySlug: slug}
	if items != nil {
		r.RequiredBuilds = &items
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

func TestRepoRequiredBuildCommandTree(t *testing.T) {
	cmd := RepoRequiredBuildCmd()
	if cmd.Use != "required-build" {
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

func TestRequiredBuildCommandFlags(t *testing.T) {
	tests := []struct {
		name  string
		cmd   *cobra.Command
		flags []string
	}{
		{"get", GetRequiredBuildCmd(), []string{"repositorySlug", "output", "input"}},
		{"create", CreateRequiredBuildCmd(), []string{"input", "output"}},
		{"update", UpdateRequiredBuildCmd(), []string{"input", "output"}},
		{"delete", DeleteRequiredBuildCmd(), []string{"input", "projectKey", "repositorySlug", "ids"}},
		{"diff", DiffRequiredBuildCmd(), []string{"source", "target", "output", "apply", "force-update", "apply-result-out", "apply-rollback-out", "rollback", "quiet"}},
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

func TestRequiredBuildValidation(t *testing.T) {
	withTestConfig(t)

	emptyFile := writeTempFile(t, "empty.yaml", "repositories: []\n")
	repoWithoutItems := writeTempFile(t, "repo.yaml", "repositories:\n  - projectKey: DEMO\n    repositorySlug: app\n")

	tests := []struct {
		name    string
		cmd     *cobra.Command
		args    []string
		wantErr string
	}{
		{"create without input", CreateRequiredBuildCmd(), nil, "--input is required"},
		{"update without input", UpdateRequiredBuildCmd(), nil, "--input is required"},
		{"create empty file", CreateRequiredBuildCmd(), []string{"--input", emptyFile}, ""},
		{"update empty file", UpdateRequiredBuildCmd(), []string{"--input", emptyFile}, ""},
		{"create without required builds", CreateRequiredBuildCmd(), []string{"--input", repoWithoutItems}, "no required-builds defined"},
		{"delete without args", DeleteRequiredBuildCmd(), nil, "either --input or --project, --repo and --ids"},
		{"delete input and flags", DeleteRequiredBuildCmd(), []string{"--input", emptyFile, "--projectKey", "DEMO"}, "cannot use --input together"},
		{"delete invalid id", DeleteRequiredBuildCmd(), []string{"--projectKey", "DEMO", "--repositorySlug", "app", "--ids", "abc"}, "invalid id"},
		{"get without args", GetRequiredBuildCmd(), nil, "please specify --repositorySlug"},
		{"get invalid identifier", GetRequiredBuildCmd(), []string{"--repositorySlug", "DEMO"}, "invalid repository identifier format"},
		{"diff missing files", DiffRequiredBuildCmd(), nil, "both --source and --target are required"},
		{"diff apply-result-out without apply", DiffRequiredBuildCmd(), []string{"--source", emptyFile, "--target", emptyFile, "--apply-result-out", "out.json"}, "--apply-result-out can only be used together with --apply"},
		{"diff rollback with apply", DiffRequiredBuildCmd(), []string{"--rollback", "plan.json", "--apply"}, "--rollback cannot be combined with --apply"},
		{"diff invalid output", DiffRequiredBuildCmd(), []string{"--source", emptyFile, "--target", emptyFile, "-o", "xml"}, "invalid output format"},
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

func TestGenerateRequiredBuildDiff(t *testing.T) {
	src := []models.ExtendedRepository{
		rbRepo("DEMO", "app", rb(int64Ptr(1), "old")),
		rbRepo("OPS", "old", rb(int64Ptr(5), "gone")),
	}
	tgt := []models.ExtendedRepository{
		rbRepo("DEMO", "app",
			rb(int64Ptr(1), "new"),
			rb(int64Ptr(2), "added"),
			rb(nil, "no-id"),
		),
		rbRepo("DEMO", "new", rb(int64Ptr(9), "brand-new")),
	}

	diff, err := generateRequiredBuildDiff(src, tgt)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(diff.Create) != 2 {
		t.Fatalf("create repos = %d, want 2", len(diff.Create))
	}
	if len(*diff.Create[0].RequiredBuilds) != 2 {
		t.Fatalf("create items = %d, want 2", len(*diff.Create[0].RequiredBuilds))
	}
	if len(diff.Update) != 1 {
		t.Fatalf("update repos = %d, want 1", len(diff.Update))
	}
	if len(diff.Delete) != 1 || diff.Delete[0].ProjectKey != "OPS" {
		t.Fatalf("unexpected delete: %+v", diff.Delete)
	}
}

func TestGenerateRequiredBuildDiff_NoItems(t *testing.T) {
	src := []models.ExtendedRepository{rbRepo("DEMO", "app")}
	tgt := []models.ExtendedRepository{rbRepo("DEMO", "app")}

	diff, err := generateRequiredBuildDiff(src, tgt)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(diff.Create) != 0 || len(diff.Update) != 0 || len(diff.Delete) != 0 {
		t.Fatalf("expected empty diff, got %+v", diff)
	}
}
