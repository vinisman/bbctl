package utils

import (
	"fmt"
	"strings"

	"github.com/vinisman/bitbucket-sdk-go/openapi"
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

func SingleRepoSlice(r *openapi.RestRepository) []openapi.RestRepository {
	if r == nil {
		return nil
	}
	return []openapi.RestRepository{*r}
}
