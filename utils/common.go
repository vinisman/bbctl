package utils

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

func SafeString(s *string) string {
	if s == nil {
		return "<nil>"
	}
	return *s
}

func SafeInterface(v interface{}) string {
	if v == nil {
		return "<nil>"
	}
	return fmt.Sprintf("%v", v)
}

func SafeInt32(i *int32) string {
	if i == nil {
		return "<nil>"
	}
	return fmt.Sprintf("%d", *i)
}

func Int32PtrToString(v *int32) string {
	if v == nil {
		return "<nil>"
	}
	return fmt.Sprintf("%d", *v)
}

func ParseColumns(columns string) []string {
	if columns == "" {
		return nil
	}
	parts := strings.Split(columns, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if s := strings.TrimSpace(p); s != "" {
			out = append(out, s)
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

// ParseYAMLFile reads YAML file and unmarshals into provided struct pointer
func ParseYAMLFile[T any](filePath string, out *T) error {
	if !isSafePath(filePath) {
		return fmt.Errorf("invalid file path")
	}

	cleanPath := filepath.Clean(filePath)
	if strings.Contains(cleanPath, "..") || strings.HasPrefix(cleanPath, "/") {
		return fmt.Errorf("invalid file path")
	}

	data, err := os.ReadFile(cleanPath)
	if err != nil {
		return fmt.Errorf("failed to read file %s: %w", filePath, err)
	}

	if err := yaml.Unmarshal(data, out); err != nil {
		return fmt.Errorf("failed to parse YAML file %s: %w", filePath, err)
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
