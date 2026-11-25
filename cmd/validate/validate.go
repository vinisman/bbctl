package validate

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"unicode"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

const (
	// Maximum file size for validation (100 MB)
	maxFileSize = 100 << 20
)

var (
	flagSchema  string
	flagData    string
	flagVerbose bool
	flagOutput  string
)

// errFileTooLarge is returned when an input file or stdin exceeds maxFileSize.
var errFileTooLarge = fmt.Errorf("file exceeds maximum allowed size (%d MB)", maxFileSize>>20)

// errValidationFailed marks a validation failure that has already been reported
// by formatError in the requested output format (exit code comes from main).
var errValidationFailed = errors.New("validation failed")

func NewValidateCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "validate",
		Short: "Validate a file against a JSON Schema",
		Long: `Validate a JSON or YAML file against a JSON Schema.

Examples:
  bbctl validate --schema schema.json --data data.json
  bbctl validate --schema schema.json --data data.yaml
  bbctl validate --schema schema.json --data -          # read data from stdin
  bbctl validate --schema - --data data.json            # read schema from stdin
  bbctl validate --schema schema.json --data data.json --verbose
  bbctl validate --schema schema.json --data data.json -o json`,
		RunE: func(cmd *cobra.Command, args []string) error {
			err := runValidate(cmd, args)
			// formatError has already printed the report in the requested
			// format; suppress cobra's generic "Error:" line for this case
			// only (flag/usage errors still print as before). The flag is set
			// once, right before returning — safe: the command runs once per
			// process.
			if errors.Is(err, errValidationFailed) {
				cmd.SilenceErrors = true
			}
			return err
		},
		SilenceUsage: true, // Don't show usage on validation error
		// Local file validation doesn't need Bitbucket credentials (issue #89)
		Annotations: map[string]string{"offline": "true"},
	}

	cmd.Flags().StringVar(&flagSchema, "schema", "", "Path to JSON Schema file (or '-' for stdin)")
	cmd.Flags().StringVar(&flagData, "data", "", "Path to data file (JSON/YAML, or '-' for stdin)")
	cmd.Flags().BoolVar(&flagVerbose, "verbose", false, "Show detailed validation errors")
	cmd.Flags().StringVarP(&flagOutput, "output", "o", "plain", "Output format (plain, json)")

	_ = cmd.MarkFlagRequired("schema")
	_ = cmd.MarkFlagRequired("data")

	return cmd
}

func runValidate(cmd *cobra.Command, args []string) error {
	if flagOutput != "" && flagOutput != "plain" && flagOutput != "json" {
		return fmt.Errorf("invalid output format %q (allowed: plain, json)", flagOutput)
	}
	if flagSchema == "-" && flagData == "-" {
		return fmt.Errorf("--schema and --data cannot both be read from stdin")
	}

	// Load schema
	schemaData, err := readFile(flagSchema)
	if err != nil {
		return fmt.Errorf("failed to read schema: %w", err)
	}

	// Load data
	dataData, err := readFile(flagData)
	if err != nil {
		return fmt.Errorf("failed to read data: %w", err)
	}

	// Size limits for files and stdin are enforced by readFile.

	// Parse schema (only JSON, since jsonschema.Schema requires JSON)
	var schemaObj jsonschema.Schema
	if err := json.Unmarshal(schemaData, &schemaObj); err != nil {
		// If not JSON, try to convert YAML to JSON
		var yamlData any
		if yamlErr := yaml.Unmarshal(schemaData, &yamlData); yamlErr != nil {
			return fmt.Errorf("invalid schema (JSON expected): %w", err)
		}
		// Convert YAML->JSON
		jsonData, convErr := json.Marshal(yamlData)
		if convErr != nil {
			return fmt.Errorf("invalid schema (JSON expected): %w", err)
		}
		if unmarshalErr := json.Unmarshal(jsonData, &schemaObj); unmarshalErr != nil {
			return fmt.Errorf("invalid schema (JSON expected): %w", err)
		}
	}

	// Raw in-memory view of the schema (YAML is a superset of JSON). Used for
	// the "looks like a schema" check and for friendly error messages — no
	// extra file reads on the error path.
	var rawSchema map[string]any
	if unmarshalErr := yaml.Unmarshal(schemaData, &rawSchema); unmarshalErr != nil {
		return fmt.Errorf("invalid schema (JSON expected): %w", unmarshalErr)
	}
	if rawSchema == nil {
		// Empty document or non-object root (e.g. "" or "null")
		return fmt.Errorf("invalid schema: file does not look like a JSON Schema (missing JSON Schema keywords)")
	}

	// Check: the file must be a JSON Schema (contain schema keywords; an
	// empty {} is a valid schema — "accept anything")
	if !looksLikeSchema(rawSchema) {
		return fmt.Errorf("invalid schema: file does not look like a JSON Schema (missing JSON Schema keywords)")
	}

	// Resolve schema references
	// Create loader for local files
	loader := func(uri *url.URL) (*jsonschema.Schema, error) {
		// Get the file path from URI (handles file:///C:/..., drive letters, UNC)
		path := fileURIToPath(uri)

		// Resolve relative paths against the base schema directory
		if !filepath.IsAbs(path) && flagSchema != "-" && flagSchema != "" {
			baseDir := filepath.Dir(flagSchema)
			path = filepath.Join(baseDir, path)
		}

		// Normalize the path. No sandbox: $ref may legitimately point
		// outside the schema directory (e.g. ../shared/common.json).
		path = filepath.Clean(path)

		// Read and parse the schema file
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("loading %s: %w", path, err)
		}

		var schema jsonschema.Schema
		if err := json.Unmarshal(data, &schema); err != nil {
			return nil, fmt.Errorf("parsing %s: %w", path, err)
		}

		return &schema, nil
	}

	// Determine base URI for the schema: the schema file itself, or the
	// current directory when the schema comes from stdin — relative $ref
	// must resolve against something deterministic (cwd), not "/" .
	var baseURI string
	if flagSchema != "-" && flagSchema != "" {
		absPath, err := filepath.Abs(flagSchema)
		if err != nil {
			return fmt.Errorf("cannot resolve schema path: %w", err)
		}
		baseURI = pathToFileURI(absPath)
	} else {
		cwd, err := os.Getwd()
		if err != nil {
			return fmt.Errorf("cannot determine working directory: %w", err)
		}
		baseURI = dirToFileURI(cwd)
	}

	resolved, err := schemaObj.Resolve(&jsonschema.ResolveOptions{
		BaseURI: baseURI,
		Loader:  loader,
	})
	if err != nil {
		return fmt.Errorf("schema resolution failed: %w", err)
	}

	// Parse data (supports YAML and JSON)
	var data any
	if err := yaml.Unmarshal(dataData, &data); err != nil {
		return fmt.Errorf("failed to parse data (YAML/JSON): %w", err)
	}
	dataMap, _ := data.(map[string]any)

	// Validation
	if err := resolved.Validate(data); err != nil {
		return formatError(err, flagVerbose, flagOutput, rawSchema, dataMap)
	}

	// Output result
	if flagOutput == "json" {
		output := map[string]any{
			"valid":   true,
			"message": "Validation passed",
		}
		jsonData, marshalErr := json.MarshalIndent(output, "", "  ")
		if marshalErr != nil {
			// Static structure cannot fail; keep plain text as a fallback
			fmt.Println("Validation passed")
		} else {
			fmt.Println(string(jsonData))
		}
	} else {
		fmt.Println("Validation passed")
	}

	return nil
}

// formatError prints the validation failure in the requested output format and
// returns errValidationFailed (the report is already printed, so the command
// wrapper silences cobra's generic error line and main exits with code 1).
func formatError(err error, verbose bool, output string, schema map[string]any, data map[string]any) error {
	errStr := err.Error()

	if output == "json" {
		payload := map[string]any{
			"valid":   false,
			"message": "Validation failed",
			"error":   errStr,
		}
		jsonData, marshalErr := json.MarshalIndent(payload, "", "  ")
		if marshalErr != nil {
			fmt.Fprintf(os.Stderr, "validation failed: %s\n", errStr)
		} else {
			fmt.Println(string(jsonData))
		}
		return errValidationFailed
	}

	// Format error for human-readable output
	if verbose {
		// Full output with paths, but with a header
		fmt.Fprintf(os.Stderr, "Validation failed:\n\n")

		// Try to extract a user-friendly message
		msg := formatErrorMessage(errStr, schema, data)
		fmt.Fprintf(os.Stderr, "  %s\n\n", msg)

		// Add technical details
		fmt.Fprintf(os.Stderr, "  Technical details:\n  %s\n", errStr)
	} else {
		// Brief output - extract only the essence
		msg := formatErrorMessage(errStr, schema, data)
		fmt.Fprintf(os.Stderr, "Validation failed: %s\n", msg)
	}

	return errValidationFailed
}

// formatErrorMessage extracts a user-friendly error message
func formatErrorMessage(errStr string, schema map[string]any, data map[string]any) string {
	// Split by "validating" — the last part is the most specific error
	parts := strings.Split(errStr, "validating ")
	lastPart := parts[len(parts)-1]

	// If it's a oneOf error - simplify
	if strings.Contains(lastPart, "oneOf: did not validate") {
		if idx := strings.Index(lastPart, ": oneOf"); idx >= 0 {
			path := cleanPath(lastPart[:idx])

			// Try to extract allowed values from the parsed schema.
			// Error format: validating /definitions/Component/properties/type: oneOf: did not validate against 4 schemas
			allowedValues := findAllowedValues(schema, path)
			if len(allowedValues) > 0 {
				// Try to find the current value from data
				currentValue := extractCurrentValue(data, path)
				if currentValue != "" {
					return fmt.Sprintf("%s: invalid value %q (allowed: %s)", path, currentValue, strings.Join(allowedValues, ", "))
				}
				return fmt.Sprintf("%s: invalid value (allowed: %s)", path, strings.Join(allowedValues, ", "))
			}
			return fmt.Sprintf("%s: value does not match any allowed option", path)
		}
	}

	// If it's a const error - simplify
	if strings.Contains(lastPart, "const: ") {
		if idx := strings.Index(lastPart, "const: "); idx >= 0 {
			path := strings.TrimSpace(lastPart[:idx])
			valueAndExpected := lastPart[idx+7:] // after "const: "
			// Format: "value does not equal expected"
			if idx2 := strings.Index(valueAndExpected, " does not equal "); idx2 >= 0 {
				value := valueAndExpected[:idx2]
				expected := strings.TrimSpace(valueAndExpected[idx2+len(" does not equal "):])
				path = cleanPath(path)
				path = strings.TrimSuffix(path, ":")
				return fmt.Sprintf("%s: must be %q (got %q)", path, expected, value)
			}
		}
	}

	// If it's a pattern error - simplify
	if strings.Contains(lastPart, "pattern:") {
		if idx := strings.Index(lastPart, "pattern:"); idx >= 0 {
			path := strings.TrimSpace(lastPart[:idx])
			path = cleanPath(path)
			path = strings.TrimSuffix(path, ":")
			return path + ": does not match required pattern"
		}
	}

	// If it's a minimum/maximum error - simplify
	if strings.Contains(lastPart, "minimum:") || strings.Contains(lastPart, "maximum:") {
		if idx := strings.Index(lastPart, "minimum:"); idx >= 0 {
			path := strings.TrimSpace(lastPart[:idx])
			details := strings.TrimSpace(lastPart[idx+len("minimum:"):])
			path = cleanPath(path)
			path = strings.TrimSuffix(path, ":")
			return path + ": value below minimum (" + details + ")"
		}
		if idx := strings.Index(lastPart, "maximum:"); idx >= 0 {
			path := strings.TrimSpace(lastPart[:idx])
			details := strings.TrimSpace(lastPart[idx+len("maximum:"):])
			path = cleanPath(path)
			path = strings.TrimSuffix(path, ":")
			return path + ": value above maximum (" + details + ")"
		}
	}

	// If it's an enum error - simplify
	if strings.Contains(lastPart, "enum:") {
		if idx := strings.Index(lastPart, "enum:"); idx >= 0 {
			path := strings.TrimSpace(lastPart[:idx])
			details := strings.TrimSpace(lastPart[idx+5:])
			path = cleanPath(path)
			// Remove trailing colon from path if present
			path = strings.TrimSuffix(path, ":")

			// Extract value and allowed values from details
			// Format: "value does not equal any of: [opt1 opt2 opt3]"
			if idx2 := strings.Index(details, " does not equal any of: ["); idx2 >= 0 {
				value := strings.TrimSpace(details[:idx2])
				allowedStart := idx2 + len(" does not equal any of: [")
				if idx3 := strings.Index(details[allowedStart:], "]"); idx3 >= 0 {
					allowed := strings.TrimSpace(details[allowedStart : allowedStart+idx3])
					// Split by spaces and filter empty strings
					allowedParts := strings.Fields(allowed)
					allowedList := strings.Join(allowedParts, ", ")
					return fmt.Sprintf("%s: invalid value %q (allowed: %s)", path, value, allowedList)
				}
			}
			return path + ": does not match allowed enum values"
		}
	}

	// If it's a minLength/maxLength error - simplify
	if strings.Contains(lastPart, "minLength:") || strings.Contains(lastPart, "maxLength:") {
		if idx := strings.Index(lastPart, "minLength:"); idx >= 0 {
			path := strings.TrimSpace(lastPart[:idx])
			details := strings.TrimSpace(lastPart[idx+10:])
			path = cleanPath(path)
			path = strings.TrimSuffix(path, ":")
			return path + ": " + details
		}
		if idx := strings.Index(lastPart, "maxLength:"); idx >= 0 {
			path := strings.TrimSpace(lastPart[:idx])
			details := strings.TrimSpace(lastPart[idx+10:])
			path = cleanPath(path)
			path = strings.TrimSuffix(path, ":")
			return path + ": " + details
		}
	}

	// For other errors - just clean the path
	lastPart = cleanPath(lastPart)

	return lastPart
}

// cleanPath converts JSON Schema paths to user-friendly dot notation
func cleanPath(path string) string {
	path = strings.TrimSpace(path)
	path = strings.TrimPrefix(path, "/properties/")
	path = strings.TrimPrefix(path, "/definitions/")
	path = strings.ReplaceAll(path, "/properties/", ".")
	path = strings.ReplaceAll(path, "/definitions/", "")
	return strings.TrimSpace(path)
}

func pathToFileURI(absPath string) string {
	p := filepath.ToSlash(absPath)
	// Windows drive path "C:/..." must become "/C:/..." so that the URI
	// serializes as file:///C:/... (three slashes) instead of file://C:...
	if !strings.HasPrefix(p, "/") {
		p = "/" + p
	}
	return (&url.URL{Scheme: "file", Path: p}).String()
}

// dirToFileURI returns a file:// URI for a directory. The trailing slash makes
// relative references resolve inside the directory (RFC 3986 §5.3), not next
// to it.
func dirToFileURI(dir string) string {
	p := filepath.ToSlash(dir)
	if !strings.HasSuffix(p, "/") {
		p += "/"
	}
	if !strings.HasPrefix(p, "/") {
		p = "/" + p
	}
	return (&url.URL{Scheme: "file", Path: p}).String()
}

// fileURIToPath converts a file:// URI to a local filesystem path.
func fileURIToPath(u *url.URL) string {
	if u.Scheme != "file" {
		return filepath.FromSlash(u.Path)
	}

	path := u.Path
	switch {
	case len(u.Host) == 2 && u.Host[1] == ':' && isDriveLetter(u.Host[0]):
		// Broken but common form file://C:/Users/... — "C:" is the host
		path = u.Host + path
	case u.Host != "" && u.Host != "localhost":
		// UNC path: file://server/share → \\server\share
		path = `\\` + u.Host + path
	}

	path = filepath.FromSlash(path)
	// URL path of a Windows drive: /C:/Users/... → C:/Users/...
	if len(path) >= 3 && (path[0] == '/' || path[0] == '\\') &&
		path[2] == ':' && isDriveLetter(path[1]) {
		path = path[1:]
	}
	return path
}

func isDriveLetter(c byte) bool {
	return (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z')
}

func readFile(path string) ([]byte, error) {
	if path == "-" {
		// Read from stdin with a size limit (limit+1 detects overflow)
		return readAllLimit(os.Stdin, maxFileSize)
	}

	// Clean path for security
	cleanPath := filepath.Clean(path)

	// Reject traversal outside the working directory. ".." counts only as a
	// full path segment, so names like "foo..bar.json" stay valid.
	for _, segment := range strings.Split(cleanPath, string(filepath.Separator)) {
		if segment == ".." {
			return nil, fmt.Errorf("invalid path: %s", path)
		}
	}

	// Check file size before reading
	info, err := os.Stat(cleanPath)
	if err != nil {
		return nil, fmt.Errorf("cannot access file: %w", err)
	}

	if info.Size() > maxFileSize {
		return nil, errFileTooLarge
	}

	return os.ReadFile(cleanPath)
}

// readAllLimit reads at most limit bytes from r. Reading limit+1 bytes makes
// overflow detectable: more data is rejected instead of being silently
// truncated.
func readAllLimit(r io.Reader, limit int64) ([]byte, error) {
	data, err := io.ReadAll(io.LimitReader(r, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > limit {
		return nil, errFileTooLarge
	}
	return data, nil
}

// schemaKeywords are the JSON Schema keywords used to recognize a file as a
// schema (as opposed to data). Not exhaustive on purpose: any common keyword
// is enough.
var schemaKeywords = map[string]bool{
	"$schema": true, "$ref": true, "$defs": true, "$id": true,
	"definitions": true, "type": true, "types": true,
	"properties": true, "patternProperties": true, "additionalProperties": true,
	"propertyNames": true, "items": true, "prefixItems": true, "additionalItems": true,
	"allOf": true, "anyOf": true, "oneOf": true, "not": true,
	"if": true, "then": true, "else": true,
	"enum": true, "const": true, "required": true,
	"minimum": true, "maximum": true, "exclusiveMinimum": true, "exclusiveMaximum": true,
	"multipleOf": true, "minLength": true, "maxLength": true, "pattern": true,
	"format": true, "minItems": true, "maxItems": true, "uniqueItems": true,
	"minProperties": true, "maxProperties": true, "dependentRequired": true,
	"dependentSchemas": true, "contains": true, "minContains": true, "maxContains": true,
	"unevaluatedProperties": true, "unevaluatedItems": true,
}

// looksLikeSchema reports whether the parsed document contains at least one
// JSON Schema keyword. An empty {} is a valid schema ("accept anything").
func looksLikeSchema(schema map[string]any) bool {
	if len(schema) == 0 {
		return true
	}
	for key := range schema {
		if schemaKeywords[key] {
			return true
		}
	}
	return false
}

// capitalizeFirst upper-cases the first rune of s (replacement for the
// deprecated strings.Title, which upper-cases every word).
func capitalizeFirst(s string) string {
	if s == "" {
		return s
	}
	runes := []rune(s)
	runes[0] = unicode.ToUpper(runes[0])
	return string(runes)
}

// extractCurrentValue extracts the current value from the parsed data for the
// specified path. Works in memory: no file I/O on the error path.
func extractCurrentValue(data map[string]any, path string) string {
	if data == nil {
		return ""
	}

	// Convert path from schema format to data format
	// Component.type -> spec.component.type
	dataPath := schemaPathToDataPath(path)

	// Try to find value by path
	value := findValueInData(data, dataPath)
	if value != nil {
		if strVal, ok := value.(string); ok {
			return strVal
		}
		// For non-string values, return JSON representation
		jsonBytes, _ := json.Marshal(value)
		return string(jsonBytes)
	}

	return ""
}

// schemaPathToDataPath converts path from schema format to data format
// Component.type -> spec.component.type
// Build.tool -> spec.component.build.tool
func schemaPathToDataPath(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return ""
	}
	parts := strings.Split(path, ".")
	if len(parts) == 0 {
		return ""
	}

	// Mapping of type names from schema to field names in data
	typeToField := map[string]string{
		"Component":   "component",
		"Pipeline":    "pipeline",
		"Spec":        "spec",
		"Metadata":    "metadata",
		"Build":       "build",
		"Artifact":    "artifact",
		"Schedule":    "schedule",
		"Deploy":      "deploy",
		"Toolchain":   "toolchain",
		"Publication": "publication",
	}

	result := []string{"spec"} // Start with spec, as it's the root for most paths

	for i, part := range parts {
		// If it's a type name (capitalized), replace with field name
		if field, ok := typeToField[part]; ok {
			result = append(result, field)
		} else if i == len(parts)-1 {
			// Last element is the field name
			result = append(result, part)
		}
	}

	return strings.Join(result, ".")
}

// findValueInData finds a value in data by path (e.g., "spec.component.type")
func findValueInData(data map[string]any, path string) any {
	parts := strings.Split(path, ".")
	current := data

	for i, part := range parts {
		val, ok := current[part]
		if !ok {
			return nil
		}

		// If it's the last element, return the value
		if i == len(parts)-1 {
			return val
		}

		// Otherwise, continue descending
		if next, ok := val.(map[string]any); ok {
			current = next
		} else {
			return nil
		}
	}

	return nil
}

// findAllowedValues finds allowed values in the schema for the specified path
func findAllowedValues(schema map[string]any, path string) []string {
	// Split path into parts: "component.type" -> ["component", "type"]
	parts := strings.Split(path, ".")

	// Root definitions: "definitions" (draft-07) or "$defs" (2019-09+)
	definitions, ok := schema["definitions"].(map[string]any)
	if !ok {
		definitions, ok = schema["$defs"].(map[string]any)
	}
	if !ok {
		return nil
	}

	// Try to find the path in the schema
	current := findInDefinitions(definitions, parts)
	if current == nil {
		return nil
	}

	// Extract oneOf or enum values. Both come back as []any: oneOf entries
	// are objects with "const", enum entries are plain strings. Handle both
	// in one pass — a separate enum branch would be unreachable.
	if list, ok := current.([]any); ok {
		var values []string
		for _, item := range list {
			switch v := item.(type) {
			case string:
				values = append(values, v)
			case map[string]any:
				if constVal, exists := v["const"]; exists {
					if strVal, ok := constVal.(string); ok {
						values = append(values, strVal)
					}
				}
			}
		}
		return values
	}

	return nil
}

// findInDefinitions finds a value in definitions by path
func findInDefinitions(definitions map[string]any, path []string) any {
	// Determine which definition we need (the first part of the path usually points to it)
	// component.type -> Component -> properties -> type
	typeMapping := map[string]string{
		"component": "Component",
		"pipeline":  "Pipeline",
		"spec":      "Spec",
		"metadata":  "Metadata",
		"build":     "Build",
		"artifact":  "Artifact",
		"schedule":  "Schedule",
		"deploy":    "Deploy",
	}

	if len(path) == 0 {
		return nil
	}

	// Get definition name
	defName, ok := typeMapping[strings.ToLower(path[0])]
	if !ok {
		// Try to find directly with the first letter capitalized
		defName = capitalizeFirst(path[0])
	}

	def, ok := definitions[defName].(map[string]any)
	if !ok {
		return nil
	}

	// If path is short (only field name), look directly in properties
	if len(path) == 1 {
		return nil
	}

	// Go through nested properties, starting from the second path element
	current := def
	for i := 1; i < len(path); i++ {
		props, ok := current["properties"].(map[string]any)
		if !ok {
			// Check if we reached the final field with oneOf
			if i == len(path)-1 {
				if oneOf, exists := current["oneOf"]; exists {
					return oneOf
				}
				if enum, exists := current["enum"]; exists {
					return enum
				}
			}
			return nil
		}

		next, ok := props[path[i]].(map[string]any)
		if !ok {
			// Check if we reached the final field with oneOf
			if i == len(path)-1 {
				if oneOf, exists := current["oneOf"]; exists {
					return oneOf
				}
				if enum, exists := current["enum"]; exists {
					return enum
				}
			}
			return nil
		}
		current = next
	}

	// Check oneOf or enum in the final node
	if oneOf, exists := current["oneOf"]; exists {
		return oneOf
	}
	if enum, exists := current["enum"]; exists {
		return enum
	}

	return nil
}
