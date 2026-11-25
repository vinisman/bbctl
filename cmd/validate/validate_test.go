package validate

import (
	"encoding/json"
	"errors"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
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
			got := formatErrorMessage(tt.in, nil, nil)
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

	// $defs (draft 2019-09+) must be supported too
	defsSchema := map[string]any{
		"$defs": map[string]any{
			"Component": map[string]any{
				"properties": map[string]any{
					"type": map[string]any{"enum": []any{"docker"}},
				},
			},
		},
	}
	if got := findAllowedValues(defsSchema, "component.type"); len(got) != 1 || got[0] != "docker" {
		t.Fatalf("$defs values = %v, want [docker]", got)
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

func TestExtractCurrentValue(t *testing.T) {
	var dataMap map[string]any
	if err := yaml.Unmarshal([]byte("spec:\n  component:\n    type: zzz\n"), &dataMap); err != nil {
		t.Fatal(err)
	}
	if got := extractCurrentValue(dataMap, "Component.type"); got != "zzz" {
		t.Fatalf("current value = %q, want zzz", got)
	}
	if got := extractCurrentValue(nil, "Component.type"); got != "" {
		t.Fatalf("nil data = %q, want empty", got)
	}

	// Non-string values are returned as JSON
	if err := yaml.Unmarshal([]byte("spec:\n  component:\n    count: 42\n"), &dataMap); err != nil {
		t.Fatal(err)
	}
	if got := extractCurrentValue(dataMap, "Component.count"); got != "42" {
		t.Fatalf("numeric value = %q, want 42", got)
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

	// ".." inside a file name must not be treated as traversal
	dotsPath := filepath.Join(t.TempDir(), "foo..bar.json")
	if err := os.WriteFile(dotsPath, []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := readFile(dotsPath); err != nil {
		t.Fatalf("readFile with '..' in name: %v", err)
	}
}

func TestPathToFileURI(t *testing.T) {
	// Native absolute path must round-trip back to the same filesystem path.
	native, err := filepath.Abs(filepath.Join("dir", "schema.json"))
	if err != nil {
		t.Fatal(err)
	}
	uri := pathToFileURI(native)
	u, err := url.Parse(uri)
	if err != nil {
		t.Fatalf("parse %q: %v", uri, err)
	}
	if u.Scheme != "file" || u.Host != "" {
		t.Fatalf("uri = %q, want file:// with empty host", uri)
	}
	if got := fileURIToPath(u); got != native {
		t.Fatalf("round-trip = %q, want %q", got, native)
	}

	// Windows drive path must produce three slashes: file:///C:/...
	got := pathToFileURI("C:/Users/me/schema.json")
	want := "file:///C:/Users/me/schema.json"
	if got != want {
		t.Fatalf("pathToFileURI(C:/...) = %q, want %q", got, want)
	}
	u2, err := url.Parse(got)
	if err != nil {
		t.Fatalf("parse %q: %v", got, err)
	}
	if u2.Host != "" {
		t.Fatalf("host = %q, want empty (drive must not be the host)", u2.Host)
	}
	if p := fileURIToPath(u2); p != filepath.FromSlash("C:/Users/me/schema.json") {
		t.Fatalf("fileURIToPath = %q", p)
	}
}

func TestFileURIToPath(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "unix absolute",
			in:   "file:///home/me/schema.json",
			want: filepath.FromSlash("/home/me/schema.json"),
		},
		{
			name: "windows drive (correct form)",
			in:   "file:///C:/Users/me/schema.json",
			want: filepath.FromSlash("C:/Users/me/schema.json"),
		},
		{
			name: "windows drive (broken form with host)",
			in:   "file://C:/Users/me/schema.json",
			want: filepath.FromSlash("C:/Users/me/schema.json"),
		},
		{
			name: "localhost",
			in:   "file://localhost/C:/Users/me/schema.json",
			want: filepath.FromSlash("C:/Users/me/schema.json"),
		},
		{
			name: "unc",
			in:   "file://server/share/schema.json",
			want: `\\server` + filepath.FromSlash("/share/schema.json"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			u, err := url.Parse(tt.in)
			if err != nil {
				t.Fatalf("parse %q: %v", tt.in, err)
			}
			if got := fileURIToPath(u); got != tt.want {
				t.Fatalf("fileURIToPath(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

// Regression: base URI for --schema must be a valid file:// URI on all
// platforms (Windows: C:\... used to become file://C:\... → "invalid port").
// Also covers the negative path: validation failure must return
// errValidationFailed instead of calling os.Exit.
func TestRunValidateWithRefAbsolutePaths(t *testing.T) {
	dir := t.TempDir()
	commonPath := filepath.Join(dir, "common.json")
	schemaPath := filepath.Join(dir, "schema.json")
	dataPath := filepath.Join(dir, "data.json")

	if err := os.WriteFile(commonPath, []byte(`{
		"$defs": {"kind": {"type": "string", "enum": ["a", "b"]}}
	}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(schemaPath, []byte(`{
		"type": "object",
		"properties": {"kind": {"$ref": "./common.json#/$defs/kind"}},
		"required": ["kind"]
	}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dataPath, []byte(`{"kind": "a"}`), 0o600); err != nil {
		t.Fatal(err)
	}

	withFlags(t, schemaPath, dataPath)
	withOutput(t, "plain")

	stdout, stderr := captureStreams(t, func() {
		if err := runValidate(nil, nil); err != nil {
			t.Errorf("runValidate (valid data): %v", err)
		}
	})
	if stdout == "" {
		t.Error("expected success message on stdout")
	}
	if stderr != "" {
		t.Errorf("unexpected stderr: %q", stderr)
	}

	// Negative case: invalid data → errValidationFailed, report on stderr,
	// process must NOT exit
	badData := filepath.Join(dir, "bad.json")
	if err := os.WriteFile(badData, []byte(`{"kind": "zzz"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	flagData = badData // restored by withFlags cleanup

	stdout, stderr = captureStreams(t, func() {
		err := runValidate(nil, nil)
		if !errors.Is(err, errValidationFailed) {
			t.Errorf("err = %v, want errValidationFailed", err)
		}
	})
	if stdout != "" {
		t.Errorf("stdout = %q, want empty", stdout)
	}
	if !strings.Contains(stderr, "Validation failed") {
		t.Errorf("stderr = %q, want 'Validation failed' report", stderr)
	}
}

// captureStreams redirects os.Stdout/os.Stderr while fn runs. Reports are far
// below the 64 KiB pipe buffer, so no deadlock is possible; all pipe ends are
// closed before the test returns (no fd leaks).
func captureStreams(t *testing.T, fn func()) (stdout, stderr string) {
	t.Helper()
	origOut, origErr := os.Stdout, os.Stderr
	rOut, wOut, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	rErr, wErr, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout, os.Stderr = wOut, wErr
	defer func() {
		os.Stdout, os.Stderr = origOut, origErr
	}()

	fn()

	// Close writers first so ReadAll sees EOF
	wOut.Close()
	wErr.Close()
	out, _ := io.ReadAll(rOut)
	errOut, _ := io.ReadAll(rErr)
	rOut.Close()
	rErr.Close()
	return string(out), string(errOut)
}

func withOutput(t *testing.T, v string) {
	t.Helper()
	prev := flagOutput
	flagOutput = v
	t.Cleanup(func() {
		flagOutput = prev
	})
}

// formatError must not exit the process: it prints the report in the
// requested format and returns errValidationFailed (exit code comes from main).
func TestFormatError(t *testing.T) {
	validationErr := errors.New("validating /properties/type: enum: foo does not equal any of: [a b c]")
	wantMsg := `type: invalid value "foo" (allowed: a, b, c)`

	t.Run("plain", func(t *testing.T) {
		stdout, stderr := captureStreams(t, func() {
			err := formatError(validationErr, false, "plain", nil, nil)
			if !errors.Is(err, errValidationFailed) {
				t.Errorf("err = %v, want errValidationFailed", err)
			}
		})
		if stdout != "" {
			t.Errorf("stdout = %q, want empty", stdout)
		}
		want := "Validation failed: " + wantMsg + "\n"
		if stderr != want {
			t.Errorf("stderr = %q, want %q", stderr, want)
		}
	})

	t.Run("verbose", func(t *testing.T) {
		stdout, stderr := captureStreams(t, func() {
			_ = formatError(validationErr, true, "plain", nil, nil)
		})
		if stdout != "" {
			t.Errorf("stdout = %q, want empty", stdout)
		}
		want := "Validation failed:\n\n  " + wantMsg + "\n\n  Technical details:\n  " +
			validationErr.Error() + "\n"
		if stderr != want {
			t.Errorf("stderr = %q, want %q", stderr, want)
		}
	})

	t.Run("json", func(t *testing.T) {
		stdout, stderr := captureStreams(t, func() {
			err := formatError(validationErr, false, "json", nil, nil)
			if !errors.Is(err, errValidationFailed) {
				t.Errorf("err = %v, want errValidationFailed", err)
			}
		})
		if stderr != "" {
			t.Errorf("stderr = %q, want empty", stderr)
		}
		var payload map[string]any
		if err := json.Unmarshal([]byte(stdout), &payload); err != nil {
			t.Fatalf("stdout is not valid JSON: %v (%q)", err, stdout)
		}
		if payload["valid"] != false || payload["message"] != "Validation failed" {
			t.Errorf("payload = %v", payload)
		}
		if payload["error"] != validationErr.Error() {
			t.Errorf("error field = %v", payload["error"])
		}
	})
}

func TestRunValidateFlagValidation(t *testing.T) {
	t.Run("invalid output format", func(t *testing.T) {
		withOutput(t, "yaml")
		err := runValidate(nil, nil)
		if err == nil || !strings.Contains(err.Error(), "invalid output format") {
			t.Fatalf("err = %v, want invalid output format error", err)
		}
	})

	t.Run("both stdin", func(t *testing.T) {
		withFlags(t, "-", "-")
		err := runValidate(nil, nil)
		if err == nil || !strings.Contains(err.Error(), "stdin") {
			t.Fatalf("err = %v, want stdin conflict error", err)
		}
	})
}

func TestDirToFileURI(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{"/home/me", "file:///home/me/"},
		{"C:/Users/me", "file:///C:/Users/me/"},
		{"/", "file:///"},
	}
	for _, tt := range tests {
		if got := dirToFileURI(tt.in); got != tt.want {
			t.Errorf("dirToFileURI(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

// Schema from stdin with a relative $ref: refs must resolve against the
// current working directory (regression: used to resolve to "/common.json").
func TestRunValidateSchemaFromStdinRelativeRef(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "common.json"),
		[]byte(`{"type": "string", "enum": ["a"]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	dataPath := filepath.Join(dir, "data.json")
	if err := os.WriteFile(dataPath, []byte(`{"kind": "a"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Chdir(dir)

	schema := `{"type": "object", "properties": {"kind": {"$ref": "./common.json"}}, "required": ["kind"]}`
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.WriteString(schema); err != nil {
		t.Fatal(err)
	}
	w.Close() // EOF for the reader; schema is tiny, no pipe deadlock
	origStdin := os.Stdin
	os.Stdin = r
	t.Cleanup(func() {
		os.Stdin = origStdin
		r.Close()
	})

	withFlags(t, "-", dataPath)
	withOutput(t, "plain")

	_, stderr := captureStreams(t, func() {
		if err := runValidate(nil, nil); err != nil {
			t.Errorf("runValidate: %v", err)
		}
	})
	if stderr != "" {
		t.Errorf("unexpected stderr: %q", stderr)
	}
}

func TestReadAllLimit(t *testing.T) {
	data, err := readAllLimit(strings.NewReader("hello"), 10)
	if err != nil || string(data) != "hello" {
		t.Fatalf("data = %q, err = %v", data, err)
	}
	if _, err := readAllLimit(strings.NewReader("0123456789"), 5); !errors.Is(err, errFileTooLarge) {
		t.Fatalf("err = %v, want errFileTooLarge", err)
	}
}

func TestLooksLikeSchema(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want bool
	}{
		{"type", `{"type": "object"}`, true},
		{"ref only", `{"$ref": "./common.json"}`, true},
		{"allOf only", `{"allOf": []}`, true},
		{"empty schema", `{}`, true},
		{"data file", `{"name": "x", "count": 1}`, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var m map[string]any
			if err := yaml.Unmarshal([]byte(tt.in), &m); err != nil {
				t.Fatal(err)
			}
			if got := looksLikeSchema(m); got != tt.want {
				t.Fatalf("looksLikeSchema(%s) = %v, want %v", tt.in, got, tt.want)
			}
		})
	}
}
