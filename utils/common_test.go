package utils

import (
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	"github.com/vinisman/bbctl/internal/config"
)

func setTestLogger(t *testing.T) {
	t.Helper()
	prev := config.GlobalLogger
	config.GlobalLogger = slog.New(slog.NewTextHandler(io.Discard, nil))
	t.Cleanup(func() { config.GlobalLogger = prev })
}

func TestSafeValue(t *testing.T) {
	s := "value"
	i := 42

	if got := SafeValue(&s); got != "value" {
		t.Fatalf("SafeValue(&s) = %q, want %q", got, "value")
	}
	if got := SafeValue(&i); got != 42 {
		t.Fatalf("SafeValue(&i) = %d, want 42", got)
	}
	if got := SafeValue[string](nil); got != "" {
		t.Fatalf("SafeValue(nil string) = %q, want empty", got)
	}
	if got := SafeValue[int](nil); got != 0 {
		t.Fatalf("SafeValue(nil int) = %d, want 0", got)
	}
}

func TestSafeInterface(t *testing.T) {
	if got := SafeInterface(nil); got != "<nil>" {
		t.Fatalf("SafeInterface(nil) = %q, want %q", got, "<nil>")
	}
	if got := SafeInterface(123); got != "123" {
		t.Fatalf("SafeInterface(123) = %q, want %q", got, "123")
	}
}

func TestIntPtrToString(t *testing.T) {
	i32 := int32(7)
	i64 := int64(9)

	if got := Int32PtrToString(nil); got != "<nil>" {
		t.Fatalf("Int32PtrToString(nil) = %q", got)
	}
	if got := Int32PtrToString(&i32); got != "7" {
		t.Fatalf("Int32PtrToString = %q, want 7", got)
	}
	if got := Int64PtrToString(nil); got != "<nil>" {
		t.Fatalf("Int64PtrToString(nil) = %q", got)
	}
	if got := Int64PtrToString(&i64); got != "9" {
		t.Fatalf("Int64PtrToString = %q, want 9", got)
	}
}

func TestParseColumns(t *testing.T) {
	tests := []struct {
		in   string
		want []string
	}{
		{"", nil},
		{"DEMO, OPS ,, lib", []string{"DEMO", "OPS", "lib"}},
		{"DEMO", []string{"DEMO"}},
	}
	for _, tt := range tests {
		got := ParseColumns(tt.in)
		if len(got) != len(tt.want) {
			t.Fatalf("ParseColumns(%q) = %v, want %v", tt.in, got, tt.want)
		}
		for i := range got {
			if got[i] != tt.want[i] {
				t.Fatalf("ParseColumns(%q) = %v, want %v", tt.in, got, tt.want)
			}
		}
	}
}

func TestParseColumnsToLower(t *testing.T) {
	got := ParseColumnsToLower(" Name , SLUG ,, ")
	want := []string{"name", "slug"}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range got {
		if got[i] != want[i] {
			t.Fatalf("got %v, want %v", got, want)
		}
	}
	if ParseColumnsToLower("") != nil {
		t.Fatal("empty input should return nil")
	}
}

func TestHasOption(t *testing.T) {
	tests := []struct {
		options string
		target  string
		want    bool
	}{
		{"repository,webhooks", "webhooks", true},
		{" Repository , DefaultBranch ", "defaultbranch", true},
		{"", "repository", false},
		{"repository", "webhooks", false},
	}
	for _, tt := range tests {
		if got := HasOption(tt.options, tt.target); got != tt.want {
			t.Fatalf("HasOption(%q, %q) = %v, want %v", tt.options, tt.target, got, tt.want)
		}
	}
}

func TestOptionalHelpers(t *testing.T) {
	if OptionalString("") != nil {
		t.Fatal("OptionalString(\"\") should be nil")
	}
	if got := OptionalString("x"); got == nil || *got != "x" {
		t.Fatalf("OptionalString(\"x\") = %v", got)
	}
	if got := OptionalBool(false); got == nil || *got {
		t.Fatalf("OptionalBool(false) = %v", got)
	}
}

type parsePayload struct {
	Items []string `json:"items" yaml:"items"`
}

func TestParseFile(t *testing.T) {
	setTestLogger(t)
	dir := t.TempDir()

	yamlPath := filepath.Join(dir, "data.yaml")
	if err := os.WriteFile(yamlPath, []byte("items:\n  - a\n  - b\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	jsonPath := filepath.Join(dir, "data.json")
	if err := os.WriteFile(jsonPath, []byte(`{"items":["c"]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	fallbackPath := filepath.Join(dir, "data.txt")
	if err := os.WriteFile(fallbackPath, []byte("items: [d]\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name string
		path string
		want []string
	}{
		{"yaml", yamlPath, []string{"a", "b"}},
		{"json", jsonPath, []string{"c"}},
		{"yaml fallback", fallbackPath, []string{"d"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var out parsePayload
			if err := ParseFile(tt.path, &out); err != nil {
				t.Fatalf("ParseFile: %v", err)
			}
			if len(out.Items) != len(tt.want) {
				t.Fatalf("got %v, want %v", out.Items, tt.want)
			}
			for i := range out.Items {
				if out.Items[i] != tt.want[i] {
					t.Fatalf("got %v, want %v", out.Items, tt.want)
				}
			}
		})
	}
}

func TestParseFile_Errors(t *testing.T) {
	setTestLogger(t)
	if err := ParseFile(filepath.Join(t.TempDir(), "missing.yaml"), &parsePayload{}); err == nil {
		t.Fatal("expected error for missing file")
	}

	badJSON := filepath.Join(t.TempDir(), "bad.json")
	if err := os.WriteFile(badJSON, []byte("{"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := ParseFile(badJSON, &parsePayload{}); err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}
