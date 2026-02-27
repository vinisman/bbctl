package validate

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

const (
	// Максимальный размер файла для валидации (100 MB)
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
		SilenceUsage: true, // Не показывать Usage при ошибке валидации
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
	// Загрузка схемы
	schemaData, err := readFile(flagSchema)
	if err != nil {
		return fmt.Errorf("failed to read schema: %w", err)
	}

	// Загрузка данных
	dataData, err := readFile(flagData)
	if err != nil {
		return fmt.Errorf("failed to read data: %w", err)
	}

	// Проверка размера данных после чтения (для stdin)
	if len(dataData) > maxFileSize {
		return errFileTooLarge
	}

	// Парсинг схемы (только JSON, т.к. jsonschema.Schema требует JSON)
	var schemaObj jsonschema.Schema
	if err := json.Unmarshal(schemaData, &schemaObj); err != nil {
		// Если не JSON, пробуем сконвертировать YAML в JSON
		var yamlData any
		if yamlErr := yaml.Unmarshal(schemaData, &yamlData); yamlErr != nil {
			return fmt.Errorf("invalid schema (JSON expected): %w", err)
		}
		// Конвертируем YAML->JSON
		jsonData, convErr := json.Marshal(yamlData)
		if convErr != nil {
			return fmt.Errorf("invalid schema (JSON expected): %w", err)
		}
		if unmarshalErr := json.Unmarshal(jsonData, &schemaObj); unmarshalErr != nil {
			return fmt.Errorf("invalid schema (JSON expected): %w", err)
		}
	}

	// Проверка: файл должен быть JSON Schema (иметь type или $schema)
	if schemaObj.Type == "" && len(schemaObj.Types) == 0 && schemaObj.Schema == "" {
		return fmt.Errorf("invalid schema: file does not look like a JSON Schema (missing 'type' or '$schema')")
	}

	// Разрешение ссылок в схеме
	resolved, err := schemaObj.Resolve(nil)
	if err != nil {
		return fmt.Errorf("schema resolution failed: %w", err)
	}

	// Парсинг данных (поддерживаем YAML и JSON)
	var data any
	if err := yaml.Unmarshal(dataData, &data); err != nil {
		return fmt.Errorf("failed to parse data (YAML/JSON): %w", err)
	}

	// Валидация
	if err := resolved.Validate(data); err != nil {
		return formatError(err, flagVerbose, flagOutput)
	}

	// Вывод результата
	if flagOutput == "json" {
		output := map[string]any{
			"valid":   true,
			"message": "Validation passed",
		}
		jsonData, _ := json.MarshalIndent(output, "", "  ")
		fmt.Println(string(jsonData))
	} else {
		fmt.Println("✓ Validation passed")
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
		// Выводим JSON и завершаем
		fmt.Println(string(jsonData))
		os.Exit(1)
	}

	// Форматируем ошибку для человека
	if verbose {
		// Полный вывод с путями
		fmt.Fprintf(os.Stderr, "Validation failed:\n%s\n", errStr)
	} else {
		// Краткий вывод - извлекаем только суть
		msg := formatErrorMessage(errStr)
		fmt.Fprintf(os.Stderr, "Validation failed: %s\n", msg)
	}

	os.Exit(1)
	return nil // Не достигаем
}

// formatErrorMessage извлекает понятное сообщение об ошибке
func formatErrorMessage(errStr string) string {
	// Разбиваем на части по "validating"
	parts := strings.Split(errStr, "validating ")
	if len(parts) == 0 {
		return errStr
	}

	// Берём последнюю часть (самая конкретная ошибка)
	lastPart := parts[len(parts)-1]

	// Если это oneOf ошибка - упрощаем
	if strings.Contains(lastPart, "oneOf: did not validate") {
		if idx := strings.Index(lastPart, ": oneOf"); idx >= 0 {
			path := lastPart[:idx]
			path = strings.TrimPrefix(path, "/properties/")
			path = strings.TrimPrefix(path, "/definitions/")
			path = strings.ReplaceAll(path, "/properties/", ".")
			path = strings.ReplaceAll(path, "/definitions/", "")
			return path + ": value does not match any allowed option"
		}
	}

	// Если это const ошибка - упрощаем
	if strings.Contains(lastPart, "const: ") {
		if idx := strings.Index(lastPart, "const: "); idx >= 0 {
			path := strings.TrimSpace(lastPart[:idx])
			valueAndExpected := lastPart[idx+7:] // после "const: "
			// Формат: "value does not equal expected"
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

	// Если это pattern ошибка - упрощаем
	if strings.Contains(lastPart, "pattern:") {
		if idx := strings.Index(lastPart, "pattern: "); idx >= 0 {
			path := lastPart[:idx]
			path = strings.TrimPrefix(path, "/properties/")
			path = strings.TrimPrefix(path, "/definitions/")
			path = strings.ReplaceAll(path, "/properties/", ".")
			path = strings.ReplaceAll(path, "/definitions/", "")
			return path + ": does not match required pattern"
		}
	}

	// Если это minimum/maximum ошибка - упрощаем
	if strings.Contains(lastPart, "minimum:") || strings.Contains(lastPart, "maximum:") {
		if idx := strings.Index(lastPart, "minimum: "); idx >= 0 {
			path := lastPart[:idx]
			details := lastPart[idx+9:]
			path = strings.TrimPrefix(path, "/properties/")
			path = strings.TrimPrefix(path, "/definitions/")
			path = strings.ReplaceAll(path, "/properties/", ".")
			path = strings.ReplaceAll(path, "/definitions/", "")
			return path + ": value below minimum (" + details + ")"
		}
		if idx := strings.Index(lastPart, "maximum: "); idx >= 0 {
			path := lastPart[:idx]
			details := lastPart[idx+9:]
			path = strings.TrimPrefix(path, "/properties/")
			path = strings.TrimPrefix(path, "/definitions/")
			path = strings.ReplaceAll(path, "/properties/", ".")
			path = strings.ReplaceAll(path, "/definitions/", "")
			return path + ": value above maximum (" + details + ")"
		}
	}

	// Для других ошибок - просто чистим путь
	lastPart = strings.TrimPrefix(lastPart, "/properties/")
	lastPart = strings.TrimPrefix(lastPart, "/definitions/")
	lastPart = strings.ReplaceAll(lastPart, "/properties/", ".")
	lastPart = strings.ReplaceAll(lastPart, "/definitions/", "")

	return lastPart
}

func readFile(path string) ([]byte, error) {
	if path == "-" {
		// Чтение из stdin с ограничением размера
		return io.ReadAll(io.LimitReader(os.Stdin, maxFileSize))
	}

	// Очистка пути для безопасности
	cleanPath := filepath.Clean(path)

	// Проверка на абсолютный путь с выходом за пределы
	if strings.Contains(cleanPath, "..") {
		return nil, fmt.Errorf("invalid path: %s", path)
	}

	// Проверка размера файла перед чтением
	info, err := os.Stat(cleanPath)
	if err != nil {
		return nil, fmt.Errorf("cannot access file: %w", err)
	}

	if info.Size() > maxFileSize {
		return nil, errFileTooLarge
	}

	return os.ReadFile(cleanPath)
}
