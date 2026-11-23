package workzone

import (
	"bytes"
	"errors"
	"log/slog"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/vinisman/bbctl/internal/models"
	workzone "github.com/vinisman/workzone-sdk-go/client"
)

func TestNormalizeSections(t *testing.T) {
	t.Run("empty disallowed", func(t *testing.T) {
		if _, err := normalizeSections(nil, false); err == nil || !strings.Contains(err.Error(), "please specify --section") {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("empty allowed means all", func(t *testing.T) {
		got, err := normalizeSections(nil, true)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		for _, section := range allSections {
			if !got[section] {
				t.Fatalf("section %q not enabled: %v", section, got)
			}
		}
	})

	t.Run("normalizes case and spaces", func(t *testing.T) {
		got, err := normalizeSections([]string{" properties ", "REVIEWERS"}, false)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(got) != 2 || !got[SectionProperties] || !got[SectionReviewers] {
			t.Fatalf("unexpected sections: %v", got)
		}
	})

	t.Run("all keyword", func(t *testing.T) {
		got, err := normalizeSections([]string{"all"}, false)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(got) != len(allSections) {
			t.Fatalf("sections = %v, want all", got)
		}
	})

	t.Run("empty string keyword", func(t *testing.T) {
		got, err := normalizeSections([]string{""}, false)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(got) != len(allSections) {
			t.Fatalf("sections = %v, want all", got)
		}
	})

	t.Run("unsupported", func(t *testing.T) {
		if _, err := normalizeSections([]string{"unknown"}, false); err == nil || !strings.Contains(err.Error(), "unsupported --section") {
			t.Fatalf("unexpected error: %v", err)
		}
	})
}

func TestMergeSection(t *testing.T) {
	base := []models.ExtendedRepository{
		{ProjectKey: "DEMO", RepositorySlug: "app"},
		{ProjectKey: "DEMO", RepositorySlug: "api", Workzone: &models.WorkzoneData{}},
		{ProjectKey: "OPS", RepositorySlug: "mon"},
	}

	t.Run("properties", func(t *testing.T) {
		props := &workzone.WorkflowProperties{}
		add := []models.ExtendedRepository{
			{ProjectKey: "DEMO", RepositorySlug: "app", Workzone: &models.WorkzoneData{WorkflowProperties: props}},
		}
		got := mergeSection(base, add, SectionProperties)
		if len(got) != len(base) {
			t.Fatalf("repos = %d, want %d", len(got), len(base))
		}
		if got[0].Workzone == nil || got[0].Workzone.WorkflowProperties != props {
			t.Fatalf("properties not merged: %+v", got[0].Workzone)
		}
		if got[1].Workzone.WorkflowProperties != nil {
			t.Fatalf("unexpected merge for api: %+v", got[1].Workzone)
		}
	})

	t.Run("reviewers replace", func(t *testing.T) {
		add := []models.ExtendedRepository{
			{ProjectKey: "DEMO", RepositorySlug: "api", Workzone: &models.WorkzoneData{
				Reviewers: []workzone.RestBranchReviewers{{}},
			}},
		}
		got := mergeSection(base, add, SectionReviewers)
		if len(got[1].Workzone.Reviewers) != 1 {
			t.Fatalf("reviewers not merged: %+v", got[1].Workzone)
		}
	})

	t.Run("signatures", func(t *testing.T) {
		add := []models.ExtendedRepository{
			{ProjectKey: "DEMO", RepositorySlug: "app", Workzone: &models.WorkzoneData{
				Signapprovers: []workzone.RestBranchSignapprovers{{}},
			}},
		}
		got := mergeSection(base, add, SectionSignatures)
		if len(got[0].Workzone.Signapprovers) != 1 {
			t.Fatalf("signapprovers not merged: %+v", got[0].Workzone)
		}
	})

	t.Run("mergerules", func(t *testing.T) {
		add := []models.ExtendedRepository{
			{ProjectKey: "DEMO", RepositorySlug: "app", Workzone: &models.WorkzoneData{
				Mergerules: []workzone.RestBranchAutoMergers{{}},
			}},
		}
		got := mergeSection(base, add, SectionMergerules)
		if len(got[0].Workzone.Mergerules) != 1 {
			t.Fatalf("mergerules not merged: %+v", got[0].Workzone)
		}
	})

	t.Run("ignores missing and nil workzone", func(t *testing.T) {
		add := []models.ExtendedRepository{
			{ProjectKey: "DEMO", RepositorySlug: "app"},
			{ProjectKey: "UNKNOWN", RepositorySlug: "repo", Workzone: &models.WorkzoneData{}},
		}
		got := mergeSection(base, add, SectionReviewers)
		if got[0].Workzone != nil {
			t.Fatalf("unexpected workzone: %+v", got[0].Workzone)
		}
		if len(got) != len(base) {
			t.Fatalf("unexpected repos: %d", len(got))
		}
	})
}

func newTestLogger() (*slog.Logger, *bytes.Buffer) {
	buf := &bytes.Buffer{}
	return slog.New(slog.NewTextHandler(buf, &slog.HandlerOptions{Level: slog.LevelDebug})), buf
}

func TestExecuteSections_Success(t *testing.T) {
	logger, buf := newTestLogger()
	repos := []models.ExtendedRepository{
		{ProjectKey: "DEMO", RepositorySlug: "app"},
		{ProjectKey: "OPS", RepositorySlug: "mon"},
	}

	called := 0
	ops := map[string]sectionOperation{
		SectionProperties: {
			execute: func([]models.ExtendedRepository) error { called++; return nil },
			message: "set properties",
		},
	}

	executeSections(logger, repos, map[string]bool{SectionProperties: true}, ops)

	if called != 1 {
		t.Fatalf("operation called %d times, want 1", called)
	}
	out := buf.String()
	if !strings.Contains(out, "Successfully set properties properties for 2 repositories") {
		t.Fatalf("missing success message: %s", out)
	}
	if !strings.Contains(out, "All 1 sections completed successfully") {
		t.Fatalf("missing completion message: %s", out)
	}
}

func TestExecuteSections_Failure(t *testing.T) {
	logger, buf := newTestLogger()
	repos := []models.ExtendedRepository{{ProjectKey: "DEMO", RepositorySlug: "app"}}

	ops := map[string]sectionOperation{
		SectionReviewers: {
			execute: func([]models.ExtendedRepository) error { return errors.New("boom") },
			message: "set reviewers",
		},
	}

	executeSections(logger, repos, map[string]bool{SectionReviewers: true}, ops)

	out := buf.String()
	if !strings.Contains(out, "boom") {
		t.Fatalf("missing error message: %s", out)
	}
	if !strings.Contains(out, "All 1 sections failed") {
		t.Fatalf("missing failure summary: %s", out)
	}
}

func TestExecuteSections_Partial(t *testing.T) {
	logger, buf := newTestLogger()
	repos := []models.ExtendedRepository{{ProjectKey: "DEMO", RepositorySlug: "app"}}

	ops := map[string]sectionOperation{
		SectionProperties: {
			execute: func([]models.ExtendedRepository) error { return nil },
			message: "set properties",
		},
		SectionReviewers: {
			execute: func([]models.ExtendedRepository) error { return errors.New("boom") },
			message: "set reviewers",
		},
	}

	executeSections(logger, repos, map[string]bool{
		SectionProperties: true,
		SectionReviewers:  true,
	}, ops)

	if !strings.Contains(buf.String(), "Completed 1/2 sections successfully") {
		t.Fatalf("missing partial summary: %s", buf.String())
	}
}

func TestExecuteSections_NoRepos(t *testing.T) {
	logger, buf := newTestLogger()
	called := false

	executeSections(logger, nil, map[string]bool{SectionProperties: true}, map[string]sectionOperation{
		SectionProperties: {
			execute: func([]models.ExtendedRepository) error { called = true; return nil },
		},
	})

	if called {
		t.Fatal("operation should not be called without repositories")
	}
	if !strings.Contains(buf.String(), "No repositories to process") {
		t.Fatalf("missing warning: %s", buf.String())
	}
}

func TestExecuteSections_NoOperations(t *testing.T) {
	logger, buf := newTestLogger()
	repos := []models.ExtendedRepository{{ProjectKey: "DEMO", RepositorySlug: "app"}}

	executeSections(logger, repos, map[string]bool{SectionProperties: true}, nil)

	if !strings.Contains(buf.String(), "No sections were processed") {
		t.Fatalf("missing warning: %s", buf.String())
	}
}

func TestRepoWorkzoneCommandTree(t *testing.T) {
	cmd := RepoWorkzoneCmd()
	if cmd.Use != "workzone" {
		t.Fatalf("Use = %q", cmd.Use)
	}
	want := []string{"get", "set", "update", "delete"}
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

func TestWorkzoneCommandFlags(t *testing.T) {
	commands := []struct {
		name  string
		flags []string
	}{
		{"get", []string{"repositorySlug", "output", "section", "input"}},
		{"set", []string{"section", "repositorySlug", "input"}},
		{"update", []string{"section", "repositorySlug", "input"}},
		{"delete", []string{"section", "repositorySlug", "input"}},
	}

	for _, tt := range commands {
		t.Run(tt.name, func(t *testing.T) {
			cmd := RepoWorkzoneCmd()
			var sub *cobra.Command
			for _, c := range cmd.Commands() {
				if c.Name() == tt.name {
					sub = c
					break
				}
			}
			if sub == nil {
				t.Fatalf("subcommand %q not found", tt.name)
			}
			for _, flag := range tt.flags {
				if sub.Flags().Lookup(flag) == nil {
					t.Fatalf("missing flag %q", flag)
				}
			}
		})
	}
}
