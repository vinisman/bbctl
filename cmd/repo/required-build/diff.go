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
		source           string
		target           string
		output           string
		apply            bool
		forceUpdate      bool
		applyResultOut   string
		applyRollbackOut string
		rollbackFile     string
		quiet            bool
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
		 - --force-update: force update section to include ALL target items whose id exists in source (even if they are identical)
         - --apply-result-out: after --apply, FILE path to save created+updated repos; format depends on -o (json/yaml)
		 - --apply-rollback-out: save a rollback plan file after successful --apply
		 - --rollback: execute a rollback plan file (reverses a previous apply)`,
		RunE: func(cmd *cobra.Command, args []string) error {
			// Rollback mode: executes a rollback plan file
			if rollbackFile != "" {
				if apply {
					return fmt.Errorf("--rollback cannot be combined with --apply")
				}
				plan, err := utils.ReadRollbackPlan(rollbackFile)
				if err != nil {
					return fmt.Errorf("failed to read rollback file: %w", err)
				}
				client, err := bitbucket.NewClient(context.Background())
				if err != nil {
					return err
				}
				if len(plan.Delete) > 0 {
					if err := client.DeleteRequiredBuilds(plan.Delete); err != nil {
						return fmt.Errorf("rollback delete failed: %w", err)
					}
				}
				if len(plan.Update) > 0 {
					if _, err := client.UpdateRequiredBuilds(plan.Update); err != nil {
						return fmt.Errorf("rollback update failed: %w", err)
					}
				}
				if len(plan.Create) > 0 {
					if _, err := client.CreateRequiredBuilds(plan.Create); err != nil {
						return fmt.Errorf("rollback create failed: %w", err)
					}
				}
				if quiet {
					return nil
				}
				if output != "" {
					return utils.PrintStructured("rollback", plan, output, "")
				}
				return nil
			}

			if source == "" || target == "" {
				return fmt.Errorf("both --source and --target are required")
			}

			// --apply-result-out is only valid with --apply
			if applyResultOut != "" && !apply {
				return fmt.Errorf("--apply-result-out can only be used together with --apply")
			}

			// No further validation: treat value as file path; write will fail if invalid

			// --apply-rollback-out is only valid with --apply
			if applyRollbackOut != "" && !apply {
				return fmt.Errorf("--apply-rollback-out can only be used together with --apply")
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
			if forceUpdate {
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

				// Optionally write rollback plan file
				if applyRollbackOut != "" {
					rollbackPlan := utils.BuildRollbackPlan(parsedSource.Repositories, *diff, updatedRepos, createdRepos, utils.RepoItemOps[openapi.RestRequiredBuildCondition, int64]{
						GetItems: func(r models.ExtendedRepository) []openapi.RestRequiredBuildCondition {
							if r.RequiredBuilds == nil {
								return nil
							}
							return *r.RequiredBuilds
						},
						SetItems: func(r *models.ExtendedRepository, items []openapi.RestRequiredBuildCondition) {
							r.RequiredBuilds = &items
						},
						GetID: func(it openapi.RestRequiredBuildCondition) (int64, bool) {
							if it.Id == nil {
								return 0, false
							}
							return *it.Id, true
						},
						Equal: bitbucket.AreRequiredBuildsEqual,
					})
					if err := utils.WriteRollbackPlan(applyRollbackOut, output, rollbackPlan); err != nil {
						return fmt.Errorf("failed to write rollback plan: %w", err)
					}
				}

				// Optionally print apply result
				if output != "" {
					if output != "yaml" && output != "json" {
						return fmt.Errorf("invalid output format: %s, allowed values: yaml, json", output)
					}
					// If applyResultOut provided, save created+updated repos to that file (format from -o)
					reposOut := append([]models.ExtendedRepository{}, updatedRepos...)
					reposOut = append(reposOut, createdRepos...)
					if applyResultOut != "" {
						if err := utils.WriteRepositoriesToFile(applyResultOut, reposOut, output); err != nil {
							return fmt.Errorf("failed to write apply result to file: %w", err)
						}
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

			// Output diff result (non-apply)
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
	cmd.Flags().BoolVar(&forceUpdate, "force-update", false, "Force update section to include ALL target items whose id exists in source")
	cmd.Flags().StringVar(&applyResultOut, "apply-result-out", "", "After --apply: FILE path to save created+updated repos; format controlled by -o (json/yaml)")
	cmd.Flags().StringVar(&applyRollbackOut, "apply-rollback-out", "", "Write rollback plan to file after successful --apply (json or yaml)")
	cmd.Flags().StringVar(&rollbackFile, "rollback", "", "Execute rollback plan from file (reverses a previous apply)")
	cmd.Flags().BoolVarP(&quiet, "quiet", "q", false, "Suppress printing rollback plan to stdout during --rollback")

	return cmd
}

// RequiredBuildDiff represents the diff result
type RequiredBuildDiff = models.RepoDiff

// generateRequiredBuildDiff compares two sets of repositories and generates diff
// repos1 is source (current state), repos2 is target (desired state)
func generateRequiredBuildDiff(repos1, repos2 []models.ExtendedRepository) (*RequiredBuildDiff, error) {
	return utils.GenerateRepoDiff(repos1, repos2, utils.RepoItemOps[openapi.RestRequiredBuildCondition, int64]{
		GetItems: func(r models.ExtendedRepository) []openapi.RestRequiredBuildCondition {
			if r.RequiredBuilds == nil {
				return nil
			}
			return *r.RequiredBuilds
		},
		SetItems: func(r *models.ExtendedRepository, items []openapi.RestRequiredBuildCondition) {
			r.RequiredBuilds = &items
		},
		GetID: func(it openapi.RestRequiredBuildCondition) (int64, bool) {
			if it.Id == nil {
				return 0, false
			}
			return *it.Id, true
		},
		Equal: bitbucket.AreRequiredBuildsEqual,
	})
}
