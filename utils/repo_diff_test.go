package utils

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/vinisman/bbctl/internal/models"
	openapi "github.com/vinisman/bitbucket-sdk-go/openapi"
)

func int64Ptr(v int64) *int64 { return &v }

func int32Ptr(v int32) *int32 { return &v }

func rb(id *int64, keys ...string) openapi.RestRequiredBuildCondition {
	return openapi.RestRequiredBuildCondition{Id: id, BuildParentKeys: keys}
}

func repoWithRB(project, slug string, items ...openapi.RestRequiredBuildCondition) models.ExtendedRepository {
	r := models.ExtendedRepository{ProjectKey: project, RepositorySlug: slug}
	if items != nil {
		r.RequiredBuilds = &items
	}
	return r
}

func testRBOps() RepoItemOps[openapi.RestRequiredBuildCondition, int64] {
	return RepoItemOps[openapi.RestRequiredBuildCondition, int64]{
		GetItems: func(r models.ExtendedRepository) []openapi.RestRequiredBuildCondition {
			if r.RequiredBuilds == nil {
				return nil
			}
			return *r.RequiredBuilds
		},
		SetItems: func(r *models.ExtendedRepository, items []openapi.RestRequiredBuildCondition) {
			r.RequiredBuilds = &items
		},
		GetID: func(it openapi.RestRequiredBuildCondition) (int64, bool) {
			if it.Id == nil {
				return 0, false
			}
			return *it.Id, true
		},
		Equal: func(a, b openapi.RestRequiredBuildCondition) bool {
			return slices.Equal(a.BuildParentKeys, b.BuildParentKeys)
		},
	}
}

func TestBuildRepoMapsAndKeys(t *testing.T) {
	src := []models.ExtendedRepository{
		{ProjectKey: "A", RepositorySlug: "b"},
		{ProjectKey: "C", RepositorySlug: "d"},
	}
	tgt := []models.ExtendedRepository{
		{ProjectKey: "C", RepositorySlug: "d"},
		{ProjectKey: "E", RepositorySlug: "f"},
	}

	sourceMap, targetMap, keys := BuildRepoMapsAndKeys(src, tgt)
	if len(sourceMap) != 2 || len(targetMap) != 2 {
		t.Fatalf("unexpected map sizes: source=%d target=%d", len(sourceMap), len(targetMap))
	}
	want := []string{"A/b", "C/d", "E/f"}
	if !slices.Equal(keys, want) {
		t.Fatalf("keys = %v, want %v", keys, want)
	}
}

func TestDiffRepoItems(t *testing.T) {
	src := repoWithRB("DEMO", "app",
		rb(int64Ptr(1), "a"),
		rb(int64Ptr(2), "b"),
	)
	tgt := repoWithRB("DEMO", "app",
		rb(int64Ptr(1), "a2"),
		rb(int64Ptr(3), "c"),
		rb(nil, "x"),
	)

	create, update, del := DiffRepoItems(src, tgt, testRBOps())

	if len(create) != 2 {
		t.Fatalf("create = %d items, want 2", len(create))
	}
	if create[0].Id == nil || *create[0].Id != 3 {
		t.Fatalf("unexpected first create item: %+v", create[0])
	}
	if create[1].Id != nil {
		t.Fatalf("item without id should be created: %+v", create[1])
	}
	if len(update) != 1 || update[0].Id == nil || *update[0].Id != 1 {
		t.Fatalf("unexpected update: %+v", update)
	}
	if len(del) != 1 || del[0].Id == nil || *del[0].Id != 2 {
		t.Fatalf("unexpected delete: %+v", del)
	}
}

func TestGenerateRepoDiff(t *testing.T) {
	src := []models.ExtendedRepository{
		repoWithRB("DEMO", "app", rb(int64Ptr(1), "old")),
		repoWithRB("OPS", "old", rb(int64Ptr(5), "gone")),
	}
	tgt := []models.ExtendedRepository{
		repoWithRB("DEMO", "app",
			rb(int64Ptr(1), "new"),
			rb(int64Ptr(2), "added"),
			rb(nil, "no-id"),
		),
		repoWithRB("DEMO", "new", rb(int64Ptr(9), "brand-new")),
	}

	diff, err := GenerateRepoDiff(src, tgt, testRBOps())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(diff.Create) != 2 {
		t.Fatalf("create repos = %d, want 2", len(diff.Create))
	}
	if diff.Create[0].ProjectKey != "DEMO" || diff.Create[0].RepositorySlug != "app" {
		t.Fatalf("unexpected create repo: %+v", diff.Create[0])
	}
	if len(*diff.Create[0].RequiredBuilds) != 2 {
		t.Fatalf("create items = %d, want 2", len(*diff.Create[0].RequiredBuilds))
	}
	if diff.Create[1].ProjectKey != "DEMO" || diff.Create[1].RepositorySlug != "new" {
		t.Fatalf("unexpected second create repo: %+v", diff.Create[1])
	}

	if len(diff.Update) != 1 || len(*diff.Update[0].RequiredBuilds) != 1 {
		t.Fatalf("unexpected update: %+v", diff.Update)
	}
	if len(diff.Delete) != 1 || diff.Delete[0].ProjectKey != "OPS" {
		t.Fatalf("unexpected delete: %+v", diff.Delete)
	}
}

func TestGenerateRepoDiff_NoItems(t *testing.T) {
	diff, err := GenerateRepoDiff(
		[]models.ExtendedRepository{{ProjectKey: "A", RepositorySlug: "b"}},
		[]models.ExtendedRepository{{ProjectKey: "A", RepositorySlug: "b"}},
		testRBOps(),
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(diff.Create) != 0 || len(diff.Update) != 0 || len(diff.Delete) != 0 {
		t.Fatalf("expected empty diff, got %+v", diff)
	}
}

func TestForceUpdateBySourceIDs(t *testing.T) {
	src := []models.ExtendedRepository{
		repoWithRB("DEMO", "app", rb(int64Ptr(1), "a"), rb(int64Ptr(2), "b")),
	}
	tgt := []models.ExtendedRepository{
		repoWithRB("DEMO", "app", rb(int64Ptr(1), "a2"), rb(int64Ptr(3), "c")),
		repoWithRB("OPS", "other", rb(int64Ptr(1), "x")),
	}

	forced := ForceUpdateBySourceIDs(src, tgt, testRBOps())
	if len(forced) != 1 {
		t.Fatalf("forced repos = %d, want 1", len(forced))
	}
	items := *forced[0].RequiredBuilds
	if len(items) != 1 || items[0].Id == nil || *items[0].Id != 1 {
		t.Fatalf("unexpected forced items: %+v", items)
	}
}

func TestBuildRollbackPlan(t *testing.T) {
	src := []models.ExtendedRepository{
		repoWithRB("DEMO", "app", rb(int64Ptr(1), "old"), rb(int64Ptr(3), "deleted")),
	}
	diff := models.RepoDiff{
		Update: []models.ExtendedRepository{
			repoWithRB("DEMO", "app", rb(int64Ptr(1), "new")),
		},
		Delete: []models.ExtendedRepository{
			repoWithRB("DEMO", "app", rb(int64Ptr(3), "deleted")),
		},
	}
	updated := []models.ExtendedRepository{
		repoWithRB("DEMO", "app", rb(int64Ptr(1), "new")),
	}
	created := []models.ExtendedRepository{
		repoWithRB("DEMO", "app", rb(int64Ptr(2), "created")),
	}

	plan := BuildRollbackPlan(src, diff, updated, created, testRBOps())

	if len(plan.Update) != 1 || (*plan.Update[0].RequiredBuilds)[0].BuildParentKeys[0] != "old" {
		t.Fatalf("unexpected rollback update: %+v", plan.Update)
	}
	if len(plan.Delete) != 1 || *(*plan.Delete[0].RequiredBuilds)[0].Id != 2 {
		t.Fatalf("unexpected rollback delete: %+v", plan.Delete)
	}
	if len(plan.Create) != 1 || *(*plan.Create[0].RequiredBuilds)[0].Id != 3 {
		t.Fatalf("unexpected rollback create: %+v", plan.Create)
	}
}

func TestGroupRepositories(t *testing.T) {
	wh1 := openapi.RestWebhook{Id: int32Ptr(5), Name: strPtr("hook-5")}
	wh2 := openapi.RestWebhook{Id: int32Ptr(4), Name: strPtr("hook-4")}

	input := []models.ExtendedRepository{
		{
			ProjectKey:     "DEMO",
			RepositorySlug: "app",
			RequiredBuilds: &[]openapi.RestRequiredBuildCondition{
				rb(int64Ptr(2), "b"),
				rb(int64Ptr(1), "a"),
			},
			Webhooks: &[]openapi.RestWebhook{wh1, wh1},
		},
		{
			ProjectKey:     "DEMO",
			RepositorySlug: "app",
			RequiredBuilds: &[]openapi.RestRequiredBuildCondition{
				rb(int64Ptr(1), "a"),
				rb(int64Ptr(3), "c"),
			},
			Webhooks: &[]openapi.RestWebhook{wh2},
		},
		{
			ProjectKey:     "OPS",
			RepositorySlug: "z",
			RequiredBuilds: &[]openapi.RestRequiredBuildCondition{rb(int64Ptr(1), "a")},
		},
	}

	got := GroupRepositories(input)
	if len(got) != 2 {
		t.Fatalf("repos = %d, want 2", len(got))
	}
	if got[0].ProjectKey != "DEMO" || got[0].RepositorySlug != "app" {
		t.Fatalf("unexpected first repo: %+v", got[0])
	}
	rbIDs := []int64{}
	for _, it := range *got[0].RequiredBuilds {
		rbIDs = append(rbIDs, *it.Id)
	}
	if !slices.Equal(rbIDs, []int64{1, 2, 3}) {
		t.Fatalf("required-build ids = %v, want [1 2 3]", rbIDs)
	}
	if len(*got[0].Webhooks) != 2 {
		t.Fatalf("webhooks = %d, want 2", len(*got[0].Webhooks))
	}
	if got[1].ProjectKey != "OPS" {
		t.Fatalf("unexpected second repo: %+v", got[1])
	}
}

func TestSortRepositoriesStable(t *testing.T) {
	input := []models.ExtendedRepository{
		{
			ProjectKey:     "B",
			RepositorySlug: "z",
			RequiredBuilds: &[]openapi.RestRequiredBuildCondition{
				rb(int64Ptr(2), "b"),
				rb(int64Ptr(1), "a"),
			},
		},
		{
			ProjectKey:     "A",
			RepositorySlug: "b",
			RequiredBuilds: &[]openapi.RestRequiredBuildCondition{
				rb(int64Ptr(1), "a"),
				rb(nil, "no-id"),
			},
		},
	}

	got := SortRepositoriesStable(input)
	if got[0].ProjectKey != "A" || got[1].ProjectKey != "B" {
		t.Fatalf("repos not sorted: %+v", got)
	}
	items := *got[0].RequiredBuilds
	if items[0].Id != nil || items[1].Id == nil || *items[1].Id != 1 {
		t.Fatalf("items not sorted with nil first: %+v", items)
	}
}

func TestParseRepositoriesFromArgs(t *testing.T) {
	tests := []struct {
		name      string
		repoIdent string
		input     string
		wantLen   int
		wantErr   bool
	}{
		{name: "empty", wantErr: true},
		{name: "comma separated", repoIdent: "DEMO/app, OPS/mon", wantLen: 2},
		{name: "invalid identifier", repoIdent: "DEMO", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repos, err := ParseRepositoriesFromArgs(tt.repoIdent, tt.input)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(repos) != tt.wantLen {
				t.Fatalf("repos = %d, want %d", len(repos), tt.wantLen)
			}
		})
	}
}

func TestParseRepositoriesFromArgs_File(t *testing.T) {
	setTestLogger(t)
	path := filepath.Join(t.TempDir(), "repos.yaml")
	if err := os.WriteFile(path, []byte("repositories:\n  - projectKey: DEMO\n    repositorySlug: app\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	repos, err := ParseRepositoriesFromArgs("ignored", path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(repos) != 1 || repos[0].ProjectKey != "DEMO" || repos[0].RepositorySlug != "app" {
		t.Fatalf("unexpected repos: %+v", repos)
	}
}

func TestRollbackPlanRoundTrip(t *testing.T) {
	plan := &models.RollbackPlan{
		Delete: []models.ExtendedRepository{repoWithRB("DEMO", "app", rb(int64Ptr(2), "b"))},
		Update: []models.ExtendedRepository{repoWithRB("DEMO", "app", rb(int64Ptr(1), "a"))},
		Create: []models.ExtendedRepository{repoWithRB("DEMO", "app", rb(int64Ptr(3), "c"))},
	}

	for _, format := range []string{"json", "yaml"} {
		t.Run(format, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "rollback."+format)
			if err := WriteRollbackPlan(path, format, plan); err != nil {
				t.Fatalf("WriteRollbackPlan: %v", err)
			}
			got, err := ReadRollbackPlan(path)
			if err != nil {
				t.Fatalf("ReadRollbackPlan: %v", err)
			}
			if len(got.Delete) != 1 || len(got.Update) != 1 || len(got.Create) != 1 {
				t.Fatalf("unexpected plan: %+v", got)
			}
			if *(*got.Update[0].RequiredBuilds)[0].Id != 1 {
				t.Fatalf("unexpected update id: %+v", got.Update)
			}
		})
	}
}

func TestReadRollbackPlan_RejectsTraversal(t *testing.T) {
	if _, err := ReadRollbackPlan("../rollback.json"); err == nil {
		t.Fatal("expected directory traversal error")
	}
}

func TestWriteRepositoriesToFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "repos.json")
	repos := []models.ExtendedRepository{
		repoWithRB("DEMO", "app", rb(int64Ptr(1), "a")),
	}
	if err := WriteRepositoriesToFile(path, repos, "json"); err != nil {
		t.Fatalf("WriteRepositoriesToFile: %v", err)
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var wrapper struct {
		Repositories []models.ExtendedRepository `json:"repositories"`
	}
	if err := json.Unmarshal(raw, &wrapper); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if len(wrapper.Repositories) != 1 || wrapper.Repositories[0].ProjectKey != "DEMO" {
		t.Fatalf("unexpected payload: %+v", wrapper)
	}
}

func strPtr(s string) *string { return &s }
