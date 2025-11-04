package workzone

import (
	"fmt"
	"log/slog"
	"strings"

	"github.com/vinisman/bbctl/internal/models"
)

// Section constants
const (
	SectionProperties = "properties"
	SectionReviewers  = "reviewers"
	SectionSignatures = "signatures"
	SectionMergerules = "mergerules"
)

var allSections = []string{SectionProperties, SectionReviewers, SectionSignatures, SectionMergerules}

// normalizeSections normalizes and validates section names
func normalizeSections(sections []string, allowEmpty bool) (map[string]bool, error) {
	normalized := map[string]bool{}

	if len(sections) == 0 {
		if !allowEmpty {
			return nil, fmt.Errorf("please specify --section (%s)", strings.Join(allSections, ", "))
		}
		// Default to all sections
		for _, s := range allSections {
			normalized[s] = true
		}
		return normalized, nil
	}

	for _, s := range sections {
		s = strings.TrimSpace(strings.ToLower(s))
		switch s {
		case SectionProperties, SectionReviewers, SectionSignatures, SectionMergerules:
			normalized[s] = true
		case "", "all":
			for _, sec := range allSections {
				normalized[sec] = true
			}
		default:
			return nil, fmt.Errorf("unsupported --section: %s (supported: %s)", s, strings.Join(allSections, ", "))
		}
	}

	return normalized, nil
}

// sectionOperation defines an operation on a section
type sectionOperation struct {
	execute func([]models.ExtendedRepository) error
	message string // success message template (e.g., "set", "updated", "deleted")
}

// executeSections executes operations for selected sections and reports results
func executeSections(
	logger *slog.Logger,
	repos []models.ExtendedRepository,
	normalized map[string]bool,
	operations map[string]sectionOperation,
) {
	if len(repos) == 0 {
		logger.Warn("No repositories to process")
		return
	}

	successCount := 0
	totalSections := 0

	// Execute operations in order
	sectionsOrder := []string{SectionProperties, SectionReviewers, SectionSignatures, SectionMergerules}
	for _, section := range sectionsOrder {
		if !normalized[section] {
			continue
		}

		op, ok := operations[section]
		if !ok {
			continue
		}

		totalSections++
		if err := op.execute(repos); err != nil {
			logger.Error(err.Error())
		} else {
			successCount++
			logger.Info(fmt.Sprintf("Successfully %s %s for %d repositories", op.message, section, len(repos)))
		}
	}

	// Report final results
	if totalSections == 0 {
		logger.Warn("No sections were processed")
	} else if successCount == totalSections {
		logger.Info(fmt.Sprintf("All %d sections completed successfully for %d repositories", totalSections, len(repos)))
	} else if successCount > 0 {
		logger.Warn(fmt.Sprintf("Completed %d/%d sections successfully for %d repositories", successCount, totalSections, len(repos)))
	} else {
		logger.Error(fmt.Sprintf("All %d sections failed for %d repositories", totalSections, len(repos)))
	}
}

// mergeSection merges workzone data from add into base for a specific section
func mergeSection(base, add []models.ExtendedRepository, kind string) []models.ExtendedRepository {
	// Index add by key/slug
	type key struct{ p, r string }
	addMap := make(map[key]models.ExtendedRepository, len(add))
	for _, it := range add {
		addMap[key{it.ProjectKey, it.RepositorySlug}] = it
	}

	out := make([]models.ExtendedRepository, len(base))
	copy(out, base)

	for i := range out {
		k := key{out[i].ProjectKey, out[i].RepositorySlug}
		src, ok := addMap[k]
		if !ok || src.Workzone == nil {
			continue
		}

		if out[i].Workzone == nil {
			out[i].Workzone = &models.WorkzoneData{}
		}

		switch kind {
		case SectionProperties:
			out[i].Workzone.WorkflowProperties = src.Workzone.WorkflowProperties
		case SectionReviewers:
			out[i].Workzone.Reviewers = src.Workzone.Reviewers
		case SectionSignatures:
			out[i].Workzone.Signapprovers = src.Workzone.Signapprovers
		case SectionMergerules:
			out[i].Workzone.Mergerules = src.Workzone.Mergerules
		}
	}

	return out
}
