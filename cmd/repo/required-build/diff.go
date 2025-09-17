package requiredbuild

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
	"github.com/vinisman/bbctl/internal/bitbucket"
	"github.com/vinisman/bbctl/internal/models"
	"github.com/vinisman/bbctl/utils"
	"github.com/vinisman/bitbucket-sdk-go/openapi"
)

func DiffRequiredBuildCmd() *cobra.Command {
	var (
		source        string
		target        string
		output        string
		apply         bool
		updateAllByID bool
	)

	cmd := &cobra.Command{
		Use:   "diff",
		Short: "Compare required-builds between two files and generate/apply diff",
		Long: `Compare required-builds between two YAML/JSON files and generate a diff with three sections:
 - create: items present in TARGET without id, and items with ids that are in TARGET but not in SOURCE
 - update: items with the same id present in both files, but with different fields
 - delete: items with ids present in SOURCE but not in TARGET

Notes:
 - Matching is done by projectKey + repositorySlug for repositories, and by numeric id for required-builds when id is present.
 - Items in TARGET without id are ALWAYS treated as creations and do not prevent deletions of SOURCE items (even if refMatcher looks similar).

Options:
 - --apply: execute the diff against Bitbucket (delete, then update, then create)
 - --update-all-by-id: force update section to include ALL target items whose id exists in source (even if they are identical)`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if source == "" || target == "" {
				return fmt.Errorf("both --source and --target are required")
			}

			// Parse source file
			var parsedSource models.RepositoryYaml
			if err := utils.ParseFile(source, &parsedSource); err != nil {
				return fmt.Errorf("failed to parse source file: %w", err)
			}

			// Parse target file
			var parsedTarget models.RepositoryYaml
			if err := utils.ParseFile(target, &parsedTarget); err != nil {
				return fmt.Errorf("failed to parse target file: %w", err)
			}

			// Generate diff
			diff, err := generateRequiredBuildDiff(parsedSource.Repositories, parsedTarget.Repositories)
			if err != nil {
				return fmt.Errorf("failed to generate diff: %w", err)
			}

			// Optionally, force update section to include all IDs present in both files
			if updateAllByID {
				// Build map of source IDs per repo key
				type void struct{}
				var empty void
				sourceIDs := make(map[string]map[int64]void, len(parsedSource.Repositories))
				for _, r := range parsedSource.Repositories {
					key := fmt.Sprintf("%s/%s", r.ProjectKey, r.RepositorySlug)
					ids := make(map[int64]void)
					if r.RequiredBuilds != nil {
						for _, rb := range *r.RequiredBuilds {
							if rb.Id != nil {
								ids[*rb.Id] = empty
							}
						}
					}
					sourceIDs[key] = ids
				}

				// Build update entries from TARGET where id exists in source
				forcedUpdate := make([]models.ExtendedRepository, 0, len(parsedTarget.Repositories))
				for _, r := range parsedTarget.Repositories {
					key := fmt.Sprintf("%s/%s", r.ProjectKey, r.RepositorySlug)
					ids := sourceIDs[key]
					if len(ids) == 0 || r.RequiredBuilds == nil {
						continue
					}
					selected := make([]openapi.RestRequiredBuildCondition, 0, len(*r.RequiredBuilds))
					for _, rb := range *r.RequiredBuilds {
						if rb.Id != nil {
							if _, ok := ids[*rb.Id]; ok {
								selected = append(selected, rb)
							}
						}
					}
					if len(selected) > 0 {
						repo := models.ExtendedRepository{
							ProjectKey:     r.ProjectKey,
							RepositorySlug: r.RepositorySlug,
						}
						repo.RequiredBuilds = &selected
						forcedUpdate = append(forcedUpdate, repo)
					}
				}
				diff.Update = forcedUpdate
			}

			if apply {
				client, err := bitbucket.NewClient(context.Background())
				if err != nil {
					return err
				}

				// Apply in order: delete -> update -> create
				if len(diff.Delete) > 0 {
					if err := client.DeleteRequiredBuilds(diff.Delete); err != nil {
						return fmt.Errorf("apply delete failed: %w", err)
					}
				}

				var updatedRepos []models.ExtendedRepository
				if len(diff.Update) > 0 {
					updatedRepos, err = client.UpdateRequiredBuilds(diff.Update)
					if err != nil {
						return fmt.Errorf("apply update failed: %w", err)
					}
				}

				var createdRepos []models.ExtendedRepository
				if len(diff.Create) > 0 {
					createdRepos, err = client.CreateRequiredBuilds(diff.Create)
					if err != nil {
						return fmt.Errorf("apply create failed: %w", err)
					}
				}

				// Optionally print apply result
				if output != "" {
					if output != "yaml" && output != "json" {
						return fmt.Errorf("invalid output format: %s, allowed values: yaml, json", output)
					}
					applyResult := map[string]interface{}{
						"updated": updatedRepos,
						"created": createdRepos,
						"deleted": diff.Delete, // echo what was requested to delete
					}
					return utils.PrintStructured("apply", applyResult, output, "")
				}
				return nil
			}

			// Output diff result
			if output != "" {
				if output != "yaml" && output != "json" {
					return fmt.Errorf("invalid output format: %s, allowed values: yaml, json", output)
				}
				return utils.PrintStructured("diff", diff, output, "")
			}

			// Default to JSON output
			return utils.PrintStructured("diff", diff, "json", "")
		},
	}

	cmd.Flags().StringVarP(&source, "source", "s", "", "Source YAML or JSON file (current state)")
	cmd.Flags().StringVarP(&target, "target", "t", "", "Target YAML or JSON file (desired state)")
	cmd.Flags().StringVarP(&output, "output", "o", "json", "Output format: yaml or json")
	cmd.Flags().BoolVarP(&apply, "apply", "a", false, "Apply the diff to Bitbucket: delete, then update, then create")
	cmd.Flags().BoolVar(&updateAllByID, "update-all-by-id", false, "Force update section to include ALL target items whose id exists in source")

	return cmd
}

// RequiredBuildDiff represents the diff result
type RequiredBuildDiff struct {
	// RequiredBuilds that exist in target but not in source (need to be created)
	Create []models.ExtendedRepository `json:"create" yaml:"create"`
	// RequiredBuilds that exist in both files but are different (need to be updated)
	Update []models.ExtendedRepository `json:"update" yaml:"update"`
	// RequiredBuilds that exist in source but not in target (need to be deleted)
	Delete []models.ExtendedRepository `json:"delete" yaml:"delete"`
}

// generateRequiredBuildDiff compares two sets of repositories and generates diff
// repos1 is source (current state), repos2 is target (desired state)
func generateRequiredBuildDiff(repos1, repos2 []models.ExtendedRepository) (*RequiredBuildDiff, error) {
	diff := &RequiredBuildDiff{
		Create: []models.ExtendedRepository{},
		Update: []models.ExtendedRepository{},
		Delete: []models.ExtendedRepository{},
	}

	// Create maps for easier lookup
	sourceMap := make(map[string]models.ExtendedRepository)
	targetMap := make(map[string]models.ExtendedRepository)

	// Index repositories by projectKey/repositorySlug
	for _, repo := range repos1 {
		key := fmt.Sprintf("%s/%s", repo.ProjectKey, repo.RepositorySlug)
		sourceMap[key] = repo
	}

	for _, repo := range repos2 {
		key := fmt.Sprintf("%s/%s", repo.ProjectKey, repo.RepositorySlug)
		targetMap[key] = repo
	}

	// Find all unique repository keys
	allKeys := make(map[string]bool)
	for key := range sourceMap {
		allKeys[key] = true
	}
	for key := range targetMap {
		allKeys[key] = true
	}

	// Process each repository
	for key := range allKeys {
		sourceRepo, existsInSource := sourceMap[key]
		targetRepo, existsInTarget := targetMap[key]

		if !existsInSource {
			// Repository only exists in target - all requiredBuilds should be created
			diff.Create = append(diff.Create, targetRepo)
			continue
		}

		if !existsInTarget {
			// Repository only exists in source - all requiredBuilds should be deleted
			diff.Delete = append(diff.Delete, sourceRepo)
			continue
		}

		// Both repositories exist - compare requiredBuilds
		rbDiff, err := compareRequiredBuilds(sourceRepo, targetRepo)
		if err != nil {
			return nil, fmt.Errorf("failed to compare requiredBuilds for %s: %w", key, err)
		}

		// Add non-empty diffs
		if len(rbDiff.Create) > 0 {
			diff.Create = append(diff.Create, models.ExtendedRepository{
				ProjectKey:     targetRepo.ProjectKey,
				RepositorySlug: targetRepo.RepositorySlug,
				RequiredBuilds: &rbDiff.Create,
			})
		}

		if len(rbDiff.Update) > 0 {
			diff.Update = append(diff.Update, models.ExtendedRepository{
				ProjectKey:     targetRepo.ProjectKey,
				RepositorySlug: targetRepo.RepositorySlug,
				RequiredBuilds: &rbDiff.Update,
			})
		}

		if len(rbDiff.Delete) > 0 {
			diff.Delete = append(diff.Delete, models.ExtendedRepository{
				ProjectKey:     sourceRepo.ProjectKey,
				RepositorySlug: sourceRepo.RepositorySlug,
				RequiredBuilds: &rbDiff.Delete,
			})
		}
	}

	return diff, nil
}

// RequiredBuildItemDiff represents diff for a single repository's requiredBuilds
type RequiredBuildItemDiff struct {
	Create []openapi.RestRequiredBuildCondition
	Update []openapi.RestRequiredBuildCondition
	Delete []openapi.RestRequiredBuildCondition
}

// compareRequiredBuilds compares requiredBuilds between two repositories
func compareRequiredBuilds(repo1, repo2 models.ExtendedRepository) (*RequiredBuildItemDiff, error) {
	diff := &RequiredBuildItemDiff{
		Create: []openapi.RestRequiredBuildCondition{},
		Update: []openapi.RestRequiredBuildCondition{},
		Delete: []openapi.RestRequiredBuildCondition{},
	}

	// Handle nil requiredBuilds
	rb1 := []openapi.RestRequiredBuildCondition{}
	rb2 := []openapi.RestRequiredBuildCondition{}

	if repo1.RequiredBuilds != nil {
		rb1 = *repo1.RequiredBuilds
	}
	if repo2.RequiredBuilds != nil {
		rb2 = *repo2.RequiredBuilds
	}

	// Create maps for easier lookup by numeric ID
	rb1ByID := make(map[int64]openapi.RestRequiredBuildCondition)
	rb2ByID := make(map[int64]openapi.RestRequiredBuildCondition)

	for _, rb := range rb1 {
		if rb.Id != nil {
			rb1ByID[*rb.Id] = rb
		}
	}

	for _, rb := range rb2 {
		if rb.Id != nil {
			rb2ByID[*rb.Id] = rb
		}
	}

	// Deletes: IDs present in source but not in target
	for id, rb := range rb1ByID {
		if _, ok := rb2ByID[id]; !ok {
			diff.Delete = append(diff.Delete, rb)
		}
	}

	// Creates: items in target without ID + IDs present in target but not in source
	for _, rb := range rb2 {
		if rb.Id == nil {
			diff.Create = append(diff.Create, rb)
			continue
		}
		if _, ok := rb1ByID[*rb.Id]; !ok {
			diff.Create = append(diff.Create, rb)
		}
	}

	// Updates: IDs present in both but with differences
	for id, rbTarget := range rb2ByID {
		if rbSource, ok := rb1ByID[id]; ok {
			if !areRequiredBuildsEqual(rbSource, rbTarget) {
				diff.Update = append(diff.Update, rbTarget)
			}
		}
	}

	return diff, nil
}

// areRequiredBuildsEqual compares two requiredBuilds for equality
func areRequiredBuildsEqual(rb1, rb2 openapi.RestRequiredBuildCondition) bool {
	return bitbucket.AreRequiredBuildsEqual(rb1, rb2)
}
