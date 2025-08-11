package utils

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strings"
	"text/tabwriter"

	"github.com/vinisman/bitbucket-sdk-go/openapi"
	"gopkg.in/yaml.v2"
)

// SafeStr safely dereferences a *string, returns empty string if nil
func SafeStr(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

func ParseColumns(cols string) []string {
	if cols == "" {
		return nil
	}
	parts := strings.Split(cols, ",")
	for i := range parts {
		parts[i] = strings.TrimSpace(parts[i])
	}
	return parts
}

func parseColumns(cols string) []string {
	if cols == "" {
		return nil
	}
	parts := strings.Split(cols, ",")
	for i := range parts {
		parts[i] = strings.TrimSpace(parts[i])
	}
	return parts
}

func DerefString(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func DerefBool(b *bool) bool {
	if b == nil {
		return false
	}
	return *b
}

func DerefInt32(i *int32) string {
	if i == nil {
		return ""
	}
	return fmt.Sprintf("%d", *i)
}

func Contains(slice []string, item string) bool {
	for _, s := range slice {
		if strings.EqualFold(s, item) {
			return true
		}
	}
	return false
}

func PrintRepos(repos []openapi.RestRepository, extraCols string, manifestFields []string, manifestData map[string]map[string]string, format string) {
	defaultCols := []string{"Name", "Archived", "State", "Project"}
	var allCols []string
	if extraCols != "" {
		extra := parseColumns(extraCols)
		allCols = append(defaultCols, extra...)
	} else {
		allCols = defaultCols
	}
	if len(manifestFields) > 0 {
		for _, mf := range manifestFields {
			allCols = append(allCols, "m_"+mf)
		}
	}

	// Формируем универсальную структуру для любого вывода
	type repoRow map[string]interface{}
	var rows []repoRow

	for _, repo := range repos {
		row := repoRow{}
		for _, col := range allCols {
			switch strings.TrimSpace(col) {
			case "Id":
				row[col] = DerefInt32(repo.Id)
			case "Name":
				row[col] = DerefString(repo.Name)
			case "Project":
				row[col] = DerefString(repo.Project.Name)
			case "State":
				row[col] = DerefString(repo.State)
			case "Archived":
				row[col] = DerefBool(repo.Archived)
			case "DefaultBranch":
				row[col] = DerefString(repo.DefaultBranch)
			case "Forkable":
				row[col] = DerefBool(repo.Forkable)
			case "Slug":
				row[col] = DerefString(repo.Slug)
			case "ScmId":
				row[col] = DerefString(repo.ScmId)
			case "Description":
				row[col] = DerefString(repo.Description)
			case "Public":
				row[col] = DerefBool(repo.Public)
			default:
				// If column from manifest add prefix
				if strings.HasPrefix(col, "m_") && manifestData != nil {
					fieldName := strings.TrimPrefix(col, "m_")
					slug := DerefString(repo.Slug)
					if m, ok := manifestData[slug]; ok {
						row[col] = m[fieldName]
					} else {
						row[col] = ""
					}
				} else {
					row[col] = ""
				}
			}
		}
		rows = append(rows, row)
	}

	switch strings.ToLower(format) {
	case "yaml":
		out, err := yaml.Marshal(rows)
		if err != nil {
			log.Fatalf("Ошибка при генерации YAML: %v", err)
		}
		fmt.Print(string(out))
	case "json":
		out, err := json.MarshalIndent(rows, "", "  ")
		if err != nil {
			log.Fatalf("Ошибка при генерации JSON: %v", err)
		}
		fmt.Print(string(out))
	default: // plain
		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		fmt.Fprintln(w, strings.Join(allCols, "\t"))
		for _, row := range rows {
			var vals []string
			for _, col := range allCols {
				vals = append(vals, fmt.Sprintf("%v", row[col]))
			}
			fmt.Fprintln(w, strings.Join(vals, "\t"))
		}
		w.Flush()
	}
}

func SingleRepoSlice(r *openapi.RestRepository) []openapi.RestRepository {
	if r == nil {
		return nil
	}
	return []openapi.RestRepository{*r}
}
