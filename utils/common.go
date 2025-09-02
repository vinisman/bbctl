package utils

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

func SafeValue[T any](v *T) T {
	var zero T
	if v == nil {
		return zero
	}
	return *v
}

func SafeInterface(v interface{}) string {
	if v == nil {
		return "<nil>"
	}
	return fmt.Sprintf("%v", v)
}

func Int32PtrToString(v *int32) string {
	if v == nil {
		return "<nil>"
	}
	return fmt.Sprintf("%d", *v)
}

func ParseColumnsToLower(columns string) []string {
	if columns == "" {
		return nil
	}
	parts := strings.Split(columns, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if s := strings.TrimSpace(p); s != "" {
			out = append(out, strings.ToLower(s))
		}
	}
	return out
}

func isSafePath(path string) bool {

	if strings.Contains(path, "..") || strings.Contains(path, "~") {
		return false
	}

	cleanPath := filepath.Clean(path)
	return !filepath.IsAbs(cleanPath) && !strings.HasPrefix(cleanPath, "../")
}

// ParseFile is a universal function that parses YAML or JSON files into the provided struct pointer
func ParseFile[T any](filePath string, out *T) error {
	if filePath == "-" {
		// Read from stdin
		data, err := os.ReadFile("/dev/stdin") // alternatively, use io.ReadAll(os.Stdin)
		if err != nil {
			return fmt.Errorf("failed to read from stdin: %w", err)
		}
		return parseData(filePath, data, out)
	}

	// Validate the file path
	if !isSafePath(filePath) {
		return fmt.Errorf("invalid file path")
	}

	cleanPath := filepath.Clean(filePath)
	data, err := os.ReadFile(cleanPath)
	if err != nil {
		return fmt.Errorf("failed to read file %s: %w", filePath, err)
	}

	// Parse the data based on extension
	return parseData(filePath, data, out)
}

// parseData parses the byte slice into the struct based on file extension
func parseData[T any](filePath string, data []byte, out *T) error {
	ext := strings.ToLower(filepath.Ext(filePath))
	switch ext {
	case ".yaml", ".yml":
		// Parse YAML
		if err := yaml.Unmarshal(data, out); err != nil {
			return fmt.Errorf("failed to parse YAML file %s: %w", filePath, err)
		}
	case ".json":
		// Parse JSON
		if err := json.Unmarshal(data, out); err != nil {
			return fmt.Errorf("failed to parse JSON file %s: %w", filePath, err)
		}
	default:
		// Fallback: try YAML
		if err := yaml.Unmarshal(data, out); err != nil {
			return fmt.Errorf("unknown format or failed to parse YAML/JSON file %s: %w", filePath, err)
		}
	}
	return nil
}

// Helpers
func OptionalString(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func OptionalBool(b bool) *bool {
	return &b
}
