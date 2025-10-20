package utils

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/vinisman/bbctl/internal/models"
	"gopkg.in/yaml.v3"
)

// BuildRepoMapsAndKeys builds source/target maps keyed by projectKey/repositorySlug
// and returns the union of keys in a deterministic slice order (insertion order: all keys from source, then new keys from target).
func BuildRepoMapsAndKeys(src, tgt []models.ExtendedRepository) (map[string]models.ExtendedRepository, map[string]models.ExtendedRepository, []string) {
	sourceMap := map[string]models.ExtendedRepository{}
	targetMap := map[string]models.ExtendedRepository{}

	keysMap := map[string]struct{}{}
	keys := []string{}

	for _, r := range src {
		key := r.ProjectKey + "/" + r.RepositorySlug
		sourceMap[key] = r
		if _, ok := keysMap[key]; !ok {
			keysMap[key] = struct{}{}
			keys = append(keys, key)
		}
	}
	for _, r := range tgt {
		key := r.ProjectKey + "/" + r.RepositorySlug
		targetMap[key] = r
		if _, ok := keysMap[key]; !ok {
			keysMap[key] = struct{}{}
			keys = append(keys, key)
		}
	}
	return sourceMap, targetMap, keys
}

// GenerateRepoDiff builds a generic repo-level diff using the provided item ops
func GenerateRepoDiff[T any, ID comparable](src, tgt []models.ExtendedRepository, ops RepoItemOps[T, ID]) (*models.RepoDiff, error) {
	diff := &models.RepoDiff{
		Create: []models.ExtendedRepository{},
		Update: []models.ExtendedRepository{},
		Delete: []models.ExtendedRepository{},
	}

	sourceMap, targetMap, keys := BuildRepoMapsAndKeys(src, tgt)

	for _, key := range keys {
		sourceRepo, inS := sourceMap[key]
		targetRepo, inT := targetMap[key]

		if !inS {
			// All items in target should be created
			items := ops.GetItems(targetRepo)
			if len(items) > 0 {
				r := models.ExtendedRepository{ProjectKey: targetRepo.ProjectKey, RepositorySlug: targetRepo.RepositorySlug}
				ops.SetItems(&r, items)
				diff.Create = append(diff.Create, r)
			}
			continue
		}
		if !inT {
			// All items in source should be deleted
			items := ops.GetItems(sourceRepo)
			if len(items) > 0 {
				r := models.ExtendedRepository{ProjectKey: sourceRepo.ProjectKey, RepositorySlug: sourceRepo.RepositorySlug}
				ops.SetItems(&r, items)
				diff.Delete = append(diff.Delete, r)
			}
			continue
		}

		// Both exist: compute per-item diff
		create, update, del := DiffRepoItems(sourceRepo, targetRepo, ops)
		if len(create) > 0 {
			r := models.ExtendedRepository{ProjectKey: targetRepo.ProjectKey, RepositorySlug: targetRepo.RepositorySlug}
			ops.SetItems(&r, create)
			diff.Create = append(diff.Create, r)
		}
		if len(update) > 0 {
			r := models.ExtendedRepository{ProjectKey: targetRepo.ProjectKey, RepositorySlug: targetRepo.RepositorySlug}
			ops.SetItems(&r, update)
			diff.Update = append(diff.Update, r)
		}
		if len(del) > 0 {
			r := models.ExtendedRepository{ProjectKey: sourceRepo.ProjectKey, RepositorySlug: sourceRepo.RepositorySlug}
			ops.SetItems(&r, del)
			diff.Delete = append(diff.Delete, r)
		}
	}

	return diff, nil
}

// RepoItemOps defines how to access, identify, and compare repository-scoped items.
// T is the item type (e.g., RestRequiredBuildCondition or RestWebhook), ID is the id type (e.g., int64 or int32).
type RepoItemOps[T any, ID comparable] struct {
	GetItems func(models.ExtendedRepository) []T
	SetItems func(*models.ExtendedRepository, []T)
	GetID    func(T) (ID, bool) // returns (id, true) if item has id, otherwise (_, false)
	Equal    func(T, T) bool    // semantic equality
}

// DiffRepoItems computes create/update/delete sets between two repositories for a given item type.
// - create: target items without ID, plus items whose ID is in target but not in source
// - update: items with IDs present in both repos but differing by Equal
// - delete: items with IDs present in source but not in target
func DiffRepoItems[T any, ID comparable](sourceRepo, targetRepo models.ExtendedRepository, ops RepoItemOps[T, ID]) (create []T, update []T, delete_ []T) {
	srcItems := ops.GetItems(sourceRepo)
	tgtItems := ops.GetItems(targetRepo)

	srcByID := map[ID]T{}
	tgtByID := map[ID]T{}

	for _, it := range srcItems {
		if id, ok := ops.GetID(it); ok {
			srcByID[id] = it
		}
	}
	for _, it := range tgtItems {
		if id, ok := ops.GetID(it); ok {
			tgtByID[id] = it
		}
	}

	// delete: IDs present in source but not in target
	for id, it := range srcByID {
		if _, ok := tgtByID[id]; !ok {
			delete_ = append(delete_, it)
		}
	}

	// create: items in target without ID + IDs present in target but not in source
	for _, it := range tgtItems {
		if _, ok := ops.GetID(it); !ok {
			create = append(create, it)
			continue
		}
		if id, _ := ops.GetID(it); id != *new(ID) { // id exists (already ensured)
			if _, ok := srcByID[id]; !ok {
				create = append(create, it)
			}
		}
	}

	// update: IDs present in both but with differences
	for id, tgt := range tgtByID {
		if src, ok := srcByID[id]; ok {
			if !ops.Equal(src, tgt) {
				update = append(update, tgt)
			}
		}
	}

	return create, update, delete_
}

// ForceUpdateBySourceIDs selects from target all items whose IDs are present in source for the same repo
func ForceUpdateBySourceIDs[T any, ID comparable](source, target []models.ExtendedRepository, ops RepoItemOps[T, ID]) []models.ExtendedRepository {
	// Build ID sets per repo from source
	sourceIDs := map[string]map[ID]struct{}{}
	for _, r := range source {
		key := r.ProjectKey + "/" + r.RepositorySlug
		ids := map[ID]struct{}{}
		for _, it := range ops.GetItems(r) {
			if id, ok := ops.GetID(it); ok {
				ids[id] = struct{}{}
			}
		}
		sourceIDs[key] = ids
	}

	// Select from target
	forced := []models.ExtendedRepository{}
	for _, r := range target {
		key := r.ProjectKey + "/" + r.RepositorySlug
		ids := sourceIDs[key]
		if len(ids) == 0 {
			continue
		}
		selected := []T{}
		for _, it := range ops.GetItems(r) {
			if id, ok := ops.GetID(it); ok {
				if _, ok2 := ids[id]; ok2 {
					selected = append(selected, it)
				}
			}
		}
		if len(selected) > 0 {
			rr := models.ExtendedRepository{ProjectKey: r.ProjectKey, RepositorySlug: r.RepositorySlug}
			ops.SetItems(&rr, selected)
			forced = append(forced, rr)
		}
	}
	return forced
}

// BuildRollbackPlan constructs a rollback plan based on source state and diff result
func BuildRollbackPlan[T any, ID comparable](source []models.ExtendedRepository, diff models.RepoDiff, updated, created []models.ExtendedRepository, ops RepoItemOps[T, ID]) *models.RollbackPlan {
	// index source by repo and id
	srcIdx := map[string]map[ID]T{}
	for _, r := range source {
		m := map[ID]T{}
		for _, it := range ops.GetItems(r) {
			if id, ok := ops.GetID(it); ok {
				m[id] = it
			}
		}
		srcIdx[r.ProjectKey+"/"+r.RepositorySlug] = m
	}

	// revert updates
	revert := []models.ExtendedRepository{}
	for _, r := range diff.Update {
		sel := []T{}
		for _, it := range ops.GetItems(r) {
			if id, ok := ops.GetID(it); ok {
				if srcIt, ok2 := srcIdx[r.ProjectKey+"/"+r.RepositorySlug][id]; ok2 {
					sel = append(sel, srcIt)
				}
			}
		}
		if len(sel) > 0 {
			rr := models.ExtendedRepository{ProjectKey: r.ProjectKey, RepositorySlug: r.RepositorySlug}
			ops.SetItems(&rr, sel)
			revert = append(revert, rr)
		}
	}

	// delete created: items from created with IDs
	del := []models.ExtendedRepository{}
	for _, r := range created {
		sel := []T{}
		for _, it := range ops.GetItems(r) {
			if _, ok := ops.GetID(it); ok {
				sel = append(sel, it)
			}
		}
		if len(sel) > 0 {
			rr := models.ExtendedRepository{ProjectKey: r.ProjectKey, RepositorySlug: r.RepositorySlug}
			ops.SetItems(&rr, sel)
			del = append(del, rr)
		}
	}

	// recreate deleted
	create := []models.ExtendedRepository{}
	if len(diff.Delete) > 0 {
		create = append(create, diff.Delete...)
	}

	return &models.RollbackPlan{Delete: del, Update: revert, Create: create}
}

// WriteRollbackPlan writes plan to file in json or yaml based on format
func WriteRollbackPlan(path, format string, plan *models.RollbackPlan) error {
	// create a sorted copy according to projectKey, repositorySlug, and item ids
	sorted := &models.RollbackPlan{
		Delete: SortRepositoriesStable(plan.Delete),
		Update: SortRepositoriesStable(plan.Update),
		Create: SortRepositoriesStable(plan.Create),
	}
	wrapper := map[string]interface{}{"rollback": sorted}
	var data []byte
	var err error
	switch strings.ToLower(format) {
	case "yaml", "yml":
		data, err = yaml.Marshal(wrapper)
	default:
		data, err = json.MarshalIndent(wrapper, "", "  ")
	}
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0600)
}

// ReadRollbackPlan reads a rollback plan from JSON or YAML file
func ReadRollbackPlan(path string) (*models.RollbackPlan, error) {
	// Validate file path to prevent directory traversal
	cleanPath := filepath.Clean(path)
	if strings.Contains(cleanPath, "..") {
		return nil, fmt.Errorf("invalid file path: directory traversal detected")
	}
	
	raw, err := os.ReadFile(cleanPath)
	if err != nil {
		return nil, err
	}
	var wrapper struct {
		Rollback models.RollbackPlan `json:"rollback" yaml:"rollback"`
	}
	if err := json.Unmarshal(raw, &wrapper); err != nil {
		if err2 := yaml.Unmarshal(raw, &wrapper); err2 != nil {
			return nil, err
		}
	}
	return &wrapper.Rollback, nil
}
