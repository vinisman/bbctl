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

var errFileTooLarge = errors.New("file exceeds maximum allowed size (100 MB)")

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
		RunE:         runValidate,
		SilenceUsage: true, // Don't show usage on validation error
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

	// Check data size after reading (for stdin)
	if len(dataData) > maxFileSize {
		return errFileTooLarge
	}

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

	// Check: file must be a JSON Schema (have type or $schema)
	if schemaObj.Type == "" && len(schemaObj.Types) == 0 && schemaObj.Schema == "" {
		return fmt.Errorf("invalid schema: file does not look like a JSON Schema (missing 'type' or '$schema')")
	}

	// Resolve schema references
	// Create loader for local files
	loader := func(uri *url.URL) (*jsonschema.Schema, error) {
		// Get the file path from URI
		path := uri.Path
		if uri.Scheme == "file" {
			// Handle file:// URIs (may have host on Windows)
			path = uri.Path
		}
		
		// Resolve relative paths against the base schema directory
		if !filepath.IsAbs(path) && flagSchema != "-" && flagSchema != "" {
			baseDir := filepath.Dir(flagSchema)
			path = filepath.Join(baseDir, path)
		}
		
		// Clean the path to prevent directory traversal
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

	// Determine base URI for the schema
	var baseURI string
	if flagSchema != "-" && flagSchema != "" {
		absPath, err := filepath.Abs(flagSchema)
		if err == nil {
			baseURI = "file://" + absPath
		}
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

	// Validation
	if err := resolved.Validate(data); err != nil {
		return formatError(err, flagVerbose, flagOutput)
	}

	// Output result
	if flagOutput == "json" {
		output := map[string]any{
			"valid":   true,
			"message": "Validation passed",
		}
		jsonData, _ := json.MarshalIndent(output, "", "  ")
		fmt.Println(string(jsonData))
	} else {
		fmt.Println("Validation passed")
	}

	return nil
}

func formatError(err error, verbose bool, output string) error {
	errStr := err.Error()

	if output == "json" {
		output := map[string]any{
			"valid":   false,
			"message": "Validation failed",
			"error":   errStr,
		}
		jsonData, marshalErr := json.MarshalIndent(output, "", "  ")
		if marshalErr != nil {
			fmt.Fprintf(os.Stderr, "validation failed: %s\n", errStr)
			os.Exit(1)
		}
		// Output JSON and exit
		fmt.Println(string(jsonData))
		os.Exit(1)
	}

	// Format error for human-readable output
	if verbose {
		// Full output with paths, but with a header
		fmt.Fprintf(os.Stderr, "Validation failed:\n\n")

		// Try to extract a user-friendly message
		msg := formatErrorMessage(errStr)
		fmt.Fprintf(os.Stderr, "  %s\n\n", msg)

		// Add technical details
		fmt.Fprintf(os.Stderr, "  Technical details:\n  %s\n", errStr)
	} else {
		// Brief output - extract only the essence
		msg := formatErrorMessage(errStr)
		fmt.Fprintf(os.Stderr, "Validation failed: %s\n", msg)
	}

	os.Exit(1)
	return nil // Never reached
}

// formatErrorMessage extracts a user-friendly error message
func formatErrorMessage(errStr string) string {
	// Split by "validating"
	parts := strings.Split(errStr, "validating ")
	if len(parts) == 0 {
		return errStr
	}

	// Take the last part (most specific error)
	lastPart := parts[len(parts)-1]

	// If it's a oneOf error - simplify
	if strings.Contains(lastPart, "oneOf: did not validate") {
		if idx := strings.Index(lastPart, ": oneOf"); idx >= 0 {
			path := lastPart[:idx]
			path = strings.TrimPrefix(path, "/properties/")
			path = strings.TrimPrefix(path, "/definitions/")
			path = strings.ReplaceAll(path, "/properties/", ".")
			path = strings.ReplaceAll(path, "/definitions/", "")

			// Try to extract allowed values from the error context
			// Error format: validating /definitions/Component/properties/type: oneOf: did not validate against 4 schemas
			// We need to find information about expected values
			// For this, we look for const value mentions in the original error
			allowedValues := extractAllowedValues(errStr, path)
			if len(allowedValues) > 0 {
				// Try to find the current value from data
				currentValue := extractCurrentValue(errStr, path)
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
				expected := strings.TrimSpace(valueAndExpected[idx2+18:])
				path = strings.TrimPrefix(path, "/properties/")
				path = strings.TrimPrefix(path, "/definitions/")
				path = strings.ReplaceAll(path, "/properties/", ".")
				path = strings.ReplaceAll(path, "/definitions/", "")
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
			details := strings.TrimSpace(lastPart[idx+10:])
			path = cleanPath(path)
			path = strings.TrimSuffix(path, ":")
			return path + ": value below minimum (" + details + ")"
		}
		if idx := strings.Index(lastPart, "maximum:"); idx >= 0 {
			path := strings.TrimSpace(lastPart[:idx])
			details := strings.TrimSpace(lastPart[idx+9:])
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
				allowedStart := idx2 + 25 // length of " does not equal any of: ["
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

	// If it's an unexpected additional properties error - simplify
	if strings.Contains(lastPart, "unexpected additional properties") {
		// Format: "root: unexpected additional properties [\"prop1\" \"prop2\"]"
		path := "root"
		if idx := strings.Index(lastPart, ":"); idx >= 0 {
			path = strings.TrimSpace(lastPart[:idx])
			path = cleanPath(path)
			path = strings.TrimSuffix(path, ":")
		}
		if path == "" {
			path = "root"
		}
		
		// Extract properties list
		if idx := strings.Index(lastPart, "["); idx >= 0 {
			endIdx := strings.Index(lastPart[idx:], "]")
			if endIdx >= 0 {
				props := lastPart[idx+1 : idx+endIdx]
				// Clean up the properties list
				props = strings.ReplaceAll(props, "\"", "")
				props = strings.TrimSpace(props)
				return fmt.Sprintf("%s: unexpected fields (allowed: none, got: %s)", path, props)
			}
		}
		return path + ": unexpected additional properties"
	}

	// For other errors - just clean the path
	lastPart = cleanPath(lastPart)

	return lastPart
}

// cleanPath converts JSON Schema paths to user-friendly dot notation
func cleanPath(path string) string {
	path = strings.TrimPrefix(path, "/properties/")
	path = strings.TrimPrefix(path, "/definitions/")
	path = strings.ReplaceAll(path, "/properties/", ".")
	path = strings.ReplaceAll(path, "/definitions/", "")
	return strings.TrimSpace(path)
}

func readFile(path string) ([]byte, error) {
	if path == "-" {
		// Read from stdin with size limit
		return io.ReadAll(io.LimitReader(os.Stdin, maxFileSize))
	}

	// Clean path for security
	cleanPath := filepath.Clean(path)

	// Check for absolute path with traversal
	if strings.Contains(cleanPath, "..") {
		return nil, fmt.Errorf("invalid path: %s", path)
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

// extractAllowedValues extracts allowed values from the schema for the specified path
func extractAllowedValues(errStr, path string) []string {
	// Read the schema again to extract allowed values
	schemaData, err := os.ReadFile(flagSchema)
	if err != nil {
		return nil
	}

	// Parse schema as YAML/JSON
	var schema map[string]any
	if err := yaml.Unmarshal(schemaData, &schema); err != nil {
		return nil
	}

	// Try to find the definition for the path
	// For example, for path "component.type" look in #/definitions/Component/properties/type
	allowedValues := findAllowedValues(schema, path)
	return allowedValues
}

// extractCurrentValue extracts the current value from data for the specified path
func extractCurrentValue(errStr, path string) string {
	// Read data
	dataData, err := os.ReadFile(flagData)
	if err != nil {
		return ""
	}

	// Parse data as YAML/JSON
	var data map[string]any
	if err := yaml.Unmarshal(dataData, &data); err != nil {
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
	if len(parts) == 0 {
		return nil
	}

	// Start from root definitions
	definitions, ok := schema["definitions"].(map[string]any)
	if !ok {
		return nil
	}

	// Try to find the path in the schema
	current := findInDefinitions(definitions, parts)
	if current == nil {
		return nil
	}

	// Extract oneOf or enum values
	if oneOf, ok := current.([]any); ok {
		var values []string
		for _, item := range oneOf {
			if itemMap, ok := item.(map[string]any); ok {
				if constVal, exists := itemMap["const"]; exists {
					if strVal, ok := constVal.(string); ok {
						values = append(values, strVal)
					}
				}
			}
		}
		return values
	}

	if enum, ok := current.([]any); ok {
		var values []string
		for _, item := range enum {
			if strVal, ok := item.(string); ok {
				values = append(values, strVal)
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
		// Try to find directly
		defName = path[0]
		// Try to capitalize
		defName = strings.Title(defName)
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
