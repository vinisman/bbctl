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

// printPlain renders tabular plain text output using reflection
func printPlain(data interface{}, columns string) error {
	val := reflect.ValueOf(data)

	if val.Kind() != reflect.Slice {
		return fmt.Errorf("plain output requires a slice, got %T", data)
	}

	cols := ParseColumns(columns)
	if len(cols) == 0 {
		cols = []string{"Name"} // fallback
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)

	titleCaser := cases.Title(language.English)

	// Header
	for i, col := range cols {
		if i > 0 {
			fmt.Fprint(w, "\t")
		}
		fmt.Fprint(w, titleCaser.String(col))
	}
	fmt.Fprintln(w)

	// Rows
	for i := 0; i < val.Len(); i++ {
		elem := val.Index(i)
		if elem.Kind() == reflect.Ptr {
			elem = elem.Elem()
		}

		for j, col := range cols {
			if j > 0 {
				fmt.Fprint(w, "\t")
			}

			field := elem.FieldByNameFunc(func(s string) bool {
				return strings.EqualFold(s, col)
			})

			if !field.IsValid() || !field.CanInterface() {
				fmt.Fprint(w, "")
				continue
			}

			value := field.Interface()

			// Handle pointers: dereference if not nil
			rv := reflect.ValueOf(value)
			if rv.Kind() == reflect.Ptr && !rv.IsNil() {
				valIface := rv.Elem().Interface()
				fmt.Fprint(w, valIface)
			} else if rv.Kind() == reflect.Ptr && rv.IsNil() {
				fmt.Fprint(w, "")
			} else {
				fmt.Fprint(w, value)
			}
		}
		fmt.Fprintln(w)
	}

	if err := w.Flush(); err != nil {
		return fmt.Errorf("failed to flush writer: %w", err)
	}
	return nil
}
