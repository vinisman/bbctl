package utils

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"reflect"
	"strings"
	"text/tabwriter"

	"github.com/vinisman/bbctl/internal/models"
	"golang.org/x/text/cases"
	"golang.org/x/text/language"
	"gopkg.in/yaml.v3"
)

func HasOption(options string, target string) bool {
	target = strings.ToLower(strings.TrimSpace(target))
	for _, o := range strings.Split(options, ",") {
		if strings.ToLower(strings.TrimSpace(o)) == target {
			return true
		}
	}
	return false
}

// PrintRepos prints a slice of Repository according to selected columns
func PrintRepos(repos []models.ExtendedRepository, columns []string) {
	if len(columns) == 0 {
		columns = []string{"Name", "Project"} // default
	}
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)

	// Header
	for i, col := range columns {
		if i > 0 {
			fmt.Fprint(w, "\t")
		}
		fmt.Fprint(w, col)
	}
	fmt.Fprintln(w)

	for _, r := range repos {
		for i, col := range columns {
			if i > 0 {
				fmt.Fprint(w, "\t")
			}
			switch strings.ToLower(col) {
			case "slug":
				if r.RestRepository.Slug != nil {
					fmt.Fprint(w, *r.RestRepository.Slug)
				}
			case "name":
				if r.RestRepository.Name != nil {
					fmt.Fprint(w, *r.RestRepository.Name)
				}
			case "id":
				if r.RestRepository.Id != nil {
					fmt.Fprint(w, *r.RestRepository.Id)
				}
			case "scmid":
				if r.RestRepository.ScmId != nil {
					fmt.Fprint(w, *r.RestRepository.ScmId)
				}
			case "state":
				if r.RestRepository.State != nil {
					fmt.Fprint(w, *r.RestRepository.State)
				}
			case "forkable":
				if r.RestRepository.Forkable != nil {
					fmt.Fprint(w, *r.RestRepository.Forkable)
				}
			case "hierarchical":
				if r.RestRepository.HierarchyId != nil {
					fmt.Fprint(w, *r.RestRepository.HierarchyId)
				}
			case "project":
				if r.RestRepository.Project != nil && r.RestRepository.Project.Name != nil {
					fmt.Fprint(w, *r.RestRepository.Project.Name)
				}
			case "defaultbranch":
				if r.RestRepository.DefaultBranch != nil {
					fmt.Fprint(w, *r.RestRepository.DefaultBranch)
				}
			default:
				fmt.Fprint(w, "")
			}
		}
		fmt.Fprintln(w)
	}

	if err := w.Flush(); err != nil {
		log.Printf("Warning: failed to flush writer: %v", err)
	}
}

// PrintStructured prints data in JSON, YAML, or plain table format
func PrintStructured(name string, data interface{}, format string, columns string) error {
	switch strings.ToLower(format) {
	case "json":
		out := map[string]interface{}{name: data}
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(out)

	case "yaml", "yml":
		out := map[string]interface{}{name: data}
		enc := yaml.NewEncoder(os.Stdout)
		defer enc.Close()
		return enc.Encode(out)

	case "plain":
		return printPlain(data, columns)

	default:
		return fmt.Errorf("unsupported format: %s", format)
	}
}

// printPlain renders tabular plain text output for nested structures with arrays
func printPlain(data interface{}, columns string) error {
	val := reflect.ValueOf(data)

	if val.Kind() != reflect.Slice {
		return fmt.Errorf("plain output requires a slice, got %T", data)
	}

	cols := ParseColumnsToLower(columns)
	if len(cols) == 0 {
		return fmt.Errorf("no columns specified")
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	titleCaser := cases.Title(language.English)

	// Parse column structure to understand nesting
	columnPaths := parseColumnPaths(cols)

	// Header
	for i, col := range cols {
		if i > 0 {
			fmt.Fprint(w, "\t")
		}
		// Display clean header name (last part of path)
		parts := strings.Split(col, ".")
		fmt.Fprint(w, titleCaser.String(parts[len(parts)-1]))
	}
	fmt.Fprintln(w)

	// Process each root item
	for i := 0; i < val.Len(); i++ {
		rootItem := val.Index(i)
		if rootItem.Kind() == reflect.Ptr && !rootItem.IsNil() {
			rootItem = rootItem.Elem()
		}

		// Generate all row combinations from nested arrays
		rows := generateRows(rootItem, columnPaths, []fieldValue{})

		// Print each row
		for _, row := range rows {
			for j, col := range cols {
				if j > 0 {
					fmt.Fprint(w, "\t")
				}
				if value, exists := row[col]; exists {
					fmt.Fprint(w, formatValue(value))
				} else {
					fmt.Fprint(w, "")
				}
			}
			fmt.Fprintln(w)
		}
	}

	return w.Flush()
}

// fieldValue represents a value for a specific column path
type fieldValue struct {
	path  string
	value interface{}
}

// rowData represents a single row of data
type rowData map[string]interface{}

// parseColumnPaths analyzes columns to identify array fields
func parseColumnPaths(columns []string) []columnInfo {
	var result []columnInfo

	for _, col := range columns {
		parts := strings.Split(col, ".")
		info := columnInfo{
			fullPath: col,
			parts:    parts,
		}

		// Identify if this column contains arrays
		for i, part := range parts {
			if i < len(parts)-1 {
				info.arrayFields = append(info.arrayFields, part)
			}
		}

		result = append(result, info)
	}

	return result
}

type columnInfo struct {
	fullPath    string
	parts       []string
	arrayFields []string
}

// generateRows recursively generates all row combinations from nested arrays
func generateRows(current reflect.Value, columnPaths []columnInfo, currentPath []fieldValue) []rowData {
	if current.Kind() == reflect.Ptr && !current.IsNil() {
		current = current.Elem()
	}

	// Get all non-array field values at current level
	nonArrayValues := getNonArrayValues(current, columnPaths, currentPath)

	// Find the next array field to process
	nextArrayField := findNextArrayField(columnPaths, currentPath)
	if nextArrayField == "" {
		// No more arrays to process, return single row
		return []rowData{createRow(nonArrayValues)}
	}

	// Get the array field
	arrayValue := getFieldValueByPath(current, nextArrayField)
	if arrayValue == nil || reflect.ValueOf(arrayValue).Kind() != reflect.Slice {
		// Array field doesn't exist or is not a slice, return row with empty array values
		return []rowData{createRow(nonArrayValues)}
	}

	arrayVal := reflect.ValueOf(arrayValue)
	var rows []rowData

	// Process each item in the array
	for i := 0; i < arrayVal.Len(); i++ {
		arrayItem := arrayVal.Index(i)
		if arrayItem.Kind() == reflect.Ptr && !arrayItem.IsNil() {
			arrayItem = arrayItem.Elem()
		}

		// Create path for current array item
		arrayPath := append(currentPath, fieldValue{
			path:  nextArrayField,
			value: nil, // We don't store the array itself, just process its items
		})

		// Recursively generate rows for nested structures
		nestedRows := generateRows(arrayItem, columnPaths, arrayPath)

		// Combine with current non-array values
		for _, nestedRow := range nestedRows {
			row := createRow(nonArrayValues)
			for k, v := range nestedRow {
				row[k] = v
			}
			rows = append(rows, row)
		}
	}

	if len(rows) == 0 {
		// If array was empty, return row with empty array values
		return []rowData{createRow(nonArrayValues)}
	}

	return rows
}

// getNonArrayValues gets values for columns that don't involve arrays at current level
func getNonArrayValues(current reflect.Value, columnPaths []columnInfo, currentPath []fieldValue) []fieldValue {
	var values []fieldValue

	for _, colInfo := range columnPaths {
		// Check if this column is relevant to current level
		if isColumnAtCurrentLevel(colInfo, currentPath) {
			// Get the field value
			fieldName := colInfo.parts[len(currentPath)]
			value := getFieldValue(current, fieldName)
			values = append(values, fieldValue{
				path:  colInfo.fullPath,
				value: value,
			})
		}
	}

	return values
}

// isColumnAtCurrentLevel checks if a column is at the current processing level
func isColumnAtCurrentLevel(colInfo columnInfo, currentPath []fieldValue) bool {
	if len(colInfo.parts) <= len(currentPath) {
		return false
	}

	// Check if the path up to current level matches
	for i, pathItem := range currentPath {
		if colInfo.parts[i] != pathItem.path {
			return false
		}
	}

	// Check if the next part is not an array field (we process arrays separately)
	nextPart := colInfo.parts[len(currentPath)]
	for _, arrayField := range colInfo.arrayFields {
		if arrayField == nextPart {
			return false
		}
	}

	return true
}

// findNextArrayField finds the next array field to process
func findNextArrayField(columnPaths []columnInfo, currentPath []fieldValue) string {
	currentLevel := len(currentPath)

	for _, colInfo := range columnPaths {
		if len(colInfo.parts) > currentLevel {
			nextPart := colInfo.parts[currentLevel]
			// Check if this part is an array field
			for _, arrayField := range colInfo.arrayFields {
				if arrayField == nextPart && currentLevel < len(arrayField) {
					return nextPart
				}
			}
		}
	}

	return ""
}

// createRow creates a row from field values
func createRow(values []fieldValue) rowData {
	row := make(rowData)
	for _, fv := range values {
		row[fv.path] = fv.value
	}
	return row
}

// getFieldValueByPath gets a field value using dot notation path
func getFieldValueByPath(v reflect.Value, path string) interface{} {
	if !v.IsValid() {
		return nil
	}

	current := v
	parts := strings.Split(path, ".")

	for _, part := range parts {
		if current.Kind() == reflect.Ptr && !current.IsNil() {
			current = current.Elem()
		}

		if current.Kind() != reflect.Struct {
			return nil
		}

		field := findFieldByName(current, part)
		if !field.IsValid() {
			return nil
		}

		current = field
	}

	if current.IsValid() && current.CanInterface() {
		return current.Interface()
	}

	return nil
}

// getFieldValue gets a field value by name (case-insensitive)
func getFieldValue(v reflect.Value, fieldName string) interface{} {
	if !v.IsValid() {
		return nil
	}

	if v.Kind() == reflect.Ptr && !v.IsNil() {
		v = v.Elem()
	}

	if v.Kind() != reflect.Struct {
		return nil
	}

	field := findFieldByName(v, fieldName)
	if !field.IsValid() {
		return nil
	}

	if field.Kind() == reflect.Ptr && !field.IsNil() {
		return field.Elem().Interface()
	}

	return field.Interface()
}

// findFieldByName finds field by name (case-insensitive)
func findFieldByName(v reflect.Value, name string) reflect.Value {
	t := v.Type()
	name = strings.ToLower(name)

	// First try exact match
	if field := v.FieldByNameFunc(func(s string) bool {
		return strings.ToLower(s) == name
	}); field.IsValid() {
		return field
	}

	// Then try case-insensitive match by iterating
	for i := 0; i < t.NumField(); i++ {
		fieldName := t.Field(i).Name
		if strings.ToLower(fieldName) == name {
			return v.Field(i)
		}
	}

	return reflect.Value{}
}

// formatValue safely formats any value for display
func formatValue(value interface{}) string {
	if value == nil {
		return ""
	}

	// Handle slices and arrays
	val := reflect.ValueOf(value)
	if val.Kind() == reflect.Slice || val.Kind() == reflect.Array {
		if val.Len() == 0 {
			return "[]"
		}
		var items []string
		for i := 0; i < val.Len(); i++ {
			items = append(items, fmt.Sprintf("%v", val.Index(i).Interface()))
		}
		return "[" + strings.Join(items, ", ") + "]"
	}

	return fmt.Sprintf("%v", value)
}
