package validate

import (
	"os"
	"path/filepath"
	"testing"
)

func withFlags(t *testing.T, schema, data string) {
	t.Helper()
	prevSchema, prevData := flagSchema, flagData
	flagSchema, flagData = schema, data
	t.Cleanup(func() {
		flagSchema, flagData = prevSchema, prevData
	})
}

func TestNewValidateCmd(t *testing.T) {
	cmd := NewValidateCmd()
	if cmd.Use != "validate" {
		t.Fatalf("Use = %q, want validate", cmd.Use)
	}
	if cmd.Annotations["offline"] != "true" {
		t.Fatalf("offline annotation = %q, want true", cmd.Annotations["offline"])
	}
	for _, name := range []string{"schema", "data", "verbose", "output"} {
		if cmd.Flags().Lookup(name) == nil {
			t.Fatalf("missing flag %q", name)
		}
	}
}

func TestCleanPath(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{"", ""},
		{"/properties/name", "name"},
		{"/definitions/Component/properties/type", "Component.type"},
		{"/properties/a/properties/b", "a.b"},
		{"  /properties/foo  ", "foo"},
	}

	for _, tt := range tests {
		if got := cleanPath(tt.in); got != tt.want {
			t.Fatalf("cleanPath(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestFormatErrorMessage(t *testing.T) {
	withFlags(t, filepath.Join(t.TempDir(), "missing-schema.json"), "")

	tests := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "pattern",
			in:   "validating /properties/key: pattern: does not match required pattern",
			want: "key: does not match required pattern",
		},
		{
			name: "minimum",
			in:   "validating /properties/count: minimum: 5",
			want: "count: value below minimum (5)",
		},
		{
			name: "maximum",
			in:   "validating /properties/count: maximum: 10",
			want: "count: value above maximum (10)",
		},
		{
			name: "enum",
			in:   "validating /properties/type: enum: foo does not equal any of: [a b c]",
			want: `type: invalid value "foo" (allowed: a, b, c)`,
		},
		{
			name: "minLength",
			in:   "validating /properties/name: minLength: string length 2 is less than minimum 3",
			want: "name: string length 2 is less than minimum 3",
		},
		{
			name: "maxLength",
			in:   "validating /properties/name: maxLength: string length 9 is more than maximum 5",
			want: "name: string length 9 is more than maximum 5",
		},
		{
			name: "oneOf without schema",
			in:   "validating /definitions/Component/properties/type: oneOf: did not validate against 4 schemas",
			want: "Component.type: value does not match any allowed option",
		},
		{
			name: "const",
			in:   "validating /properties/name: const: got does not equal want",
			want: `name: must be "want" (got "got")`,
		},
		{
			name: "plain error",
			in:   "something went wrong",
			want: "something went wrong",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := formatErrorMessage(tt.in)
			if got != tt.want {
				t.Fatalf("formatErrorMessage(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestSchemaPathToDataPath(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{"", ""},
		{"  ", ""},
		{"Component.type", "spec.component.type"},
		{"Build.tool", "spec.build.tool"},
		{"Unknown.field", "spec.field"},
	}

	for _, tt := range tests {
		if got := schemaPathToDataPath(tt.in); got != tt.want {
			t.Fatalf("schemaPathToDataPath(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestFindValueInData(t *testing.T) {
	data := map[string]any{
		"spec": map[string]any{
			"component": map[string]any{
				"type": "docker",
			},
			"name": "demo",
		},
	}

	if got := findValueInData(data, "spec.component.type"); got != "docker" {
		t.Fatalf("got %v, want docker", got)
	}
	if got := findValueInData(data, "spec.name"); got != "demo" {
		t.Fatalf("got %v, want demo", got)
	}
	if got := findValueInData(data, "spec.missing.value"); got != nil {
		t.Fatalf("got %v, want nil", got)
	}
}

func TestFindAllowedValues(t *testing.T) {
	schema := map[string]any{
		"definitions": map[string]any{
			"Component": map[string]any{
				"properties": map[string]any{
					"type": map[string]any{
						"enum": []any{"docker", "maven"},
					},
					"build": map[string]any{
						"oneOf": []any{
							map[string]any{"const": "gradle"},
							map[string]any{"const": "maven"},
							map[string]any{"description": "no const"},
						},
					},
				},
			},
		},
	}

	got := findAllowedValues(schema, "component.type")
	if len(got) != 2 || got[0] != "docker" || got[1] != "maven" {
		t.Fatalf("enum values = %v", got)
	}

	got = findAllowedValues(schema, "component.build")
	if len(got) != 2 || got[0] != "gradle" || got[1] != "maven" {
		t.Fatalf("oneOf values = %v", got)
	}

	if got := findAllowedValues(schema, "unknown.type"); got != nil {
		t.Fatalf("unknown definition = %v, want nil", got)
	}
	if got := findAllowedValues(map[string]any{}, "component.type"); got != nil {
		t.Fatalf("schema without definitions = %v, want nil", got)
	}
}

func TestFindInDefinitions(t *testing.T) {
	definitions := map[string]any{
		"Component": map[string]any{
			"properties": map[string]any{
				"type": map[string]any{"enum": []any{"a"}},
			},
		},
	}

	if got := findInDefinitions(definitions, []string{"component", "type"}); got == nil {
		t.Fatal("expected enum value")
	}
	if got := findInDefinitions(definitions, nil); got != nil {
		t.Fatalf("empty path = %v, want nil", got)
	}
	if got := findInDefinitions(definitions, []string{"component"}); got != nil {
		t.Fatalf("single element path = %v, want nil", got)
	}
}

func TestExtractAllowedValuesAndCurrentValue(t *testing.T) {
	dir := t.TempDir()
	schemaPath := filepath.Join(dir, "schema.yaml")
	dataPath := filepath.Join(dir, "data.yaml")

	if err := os.WriteFile(schemaPath, []byte(`definitions:
  Component:
    properties:
      type:
        enum: [docker, maven]
`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dataPath, []byte("spec:\n  component:\n    type: zzz\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	withFlags(t, schemaPath, dataPath)

	allowed := extractAllowedValues("", "Component.type")
	if len(allowed) != 2 || allowed[0] != "docker" || allowed[1] != "maven" {
		t.Fatalf("allowed = %v", allowed)
	}
	if got := extractCurrentValue("", "Component.type"); got != "zzz" {
		t.Fatalf("current value = %q, want zzz", got)
	}
}

func TestReadFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "data.txt")
	if err := os.WriteFile(path, []byte("hello"), 0o600); err != nil {
		t.Fatal(err)
	}

	data, err := readFile(path)
	if err != nil {
		t.Fatalf("readFile: %v", err)
	}
	if string(data) != "hello" {
		t.Fatalf("data = %q, want hello", data)
	}

	if _, err := readFile(filepath.Join(t.TempDir(), "missing.txt")); err == nil {
		t.Fatal("expected error for missing file")
	}
	if _, err := readFile("../secret.txt"); err == nil {
		t.Fatal("expected error for directory traversal")
	}
}
