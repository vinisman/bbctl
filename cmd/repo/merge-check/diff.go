package mergecheck

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
	"github.com/vinisman/bbctl/internal/bitbucket"
	"github.com/vinisman/bbctl/internal/models"
	"github.com/vinisman/bbctl/utils"
)

func DiffMergeCheckCmd() *cobra.Command {
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
		Short: "Compare ScriptRunner merge checks between two files and generate/apply diff",
		Long: `Compare ScriptRunner merge checks between two YAML/JSON files and generate a diff with three sections:
 - create: items present in TARGET without id, and items with ids that are in TARGET but not in SOURCE
 - update: items with the same id present in both files, but with different fields
 - delete: items with ids present in SOURCE but not in TARGET

Merge checks are compared by target keys only (target subset of current); the id field is ignored.
On --apply, update payloads are merged with the current state before POST, so ScriptRunner-specific
fields returned by GET are preserved.

Options:
 - --apply: execute the diff against Bitbucket (delete, then update, then create)
 - --force-update: force update section to include ALL target items whose id exists in source (even if they are identical)
 - --apply-result-out: after --apply, FILE path to save created+updated repos; format depends on -o (json/yaml)
 - --apply-rollback-out: save a rollback plan file after successful --apply
 - --rollback: execute a rollback plan file (reverses a previous apply)`,
		RunE: func(cmd *cobra.Command, args []string) error {
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
					if err := client.DeleteMergeChecks(plan.Delete); err != nil {
						return fmt.Errorf("rollback delete failed: %w", err)
					}
				}
				if len(plan.Update) > 0 {
					if _, err := client.UpsertMergeChecks(plan.Update); err != nil {
						return fmt.Errorf("rollback update failed: %w", err)
					}
				}
				if len(plan.Create) > 0 {
					if _, err := client.UpsertMergeChecks(plan.Create); err != nil {
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

			if applyResultOut != "" && !apply {
				return fmt.Errorf("--apply-result-out can only be used together with --apply")
			}
			if applyRollbackOut != "" && !apply {
				return fmt.Errorf("--apply-rollback-out can only be used together with --apply")
			}

			// Parse files
			var parsedSource models.RepositoryYaml
			if err := utils.ParseFile(source, &parsedSource); err != nil {
				return fmt.Errorf("failed to parse source file: %w", err)
			}
			var parsedTarget models.RepositoryYaml
			if err := utils.ParseFile(target, &parsedTarget); err != nil {
				return fmt.Errorf("failed to parse target file: %w", err)
			}

			diff, err := generateMergeCheckDiff(parsedSource.Repositories, parsedTarget.Repositories)
			if err != nil {
				return fmt.Errorf("failed to generate diff: %w", err)
			}

			if forceUpdate {
				diff.Update = utils.ForceUpdateBySourceIDs(parsedSource.Repositories, parsedTarget.Repositories, mergeCheckOps)
			}

			if apply {
				client, err := bitbucket.NewClient(context.Background())
				if err != nil {
					return err
				}

				if len(diff.Delete) > 0 {
					if err := client.DeleteMergeChecks(diff.Delete); err != nil {
						return fmt.Errorf("apply delete failed: %w", err)
					}
				}

				// update: merge target payload with current state so that
				// ScriptRunner fields returned by GET but not set in the target are preserved
				mergedUpdates := mergeUpdateRepos(parsedSource.Repositories, diff.Update)

				var updatedRepos []models.ExtendedRepository
				if len(mergedUpdates) > 0 {
					updatedRepos, err = client.UpsertMergeChecks(mergedUpdates)
					if err != nil {
						return fmt.Errorf("apply update failed: %w", err)
					}
				}

				var createdRepos []models.ExtendedRepository
				if len(diff.Create) > 0 {
					createdRepos, err = client.UpsertMergeChecks(diff.Create)
					if err != nil {
						return fmt.Errorf("apply create failed: %w", err)
					}
				}

				if applyRollbackOut != "" {
					rollbackPlan := utils.BuildRollbackPlan(parsedSource.Repositories, *diff, updatedRepos, createdRepos, mergeCheckOps)
					if err := utils.WriteRollbackPlan(applyRollbackOut, output, rollbackPlan); err != nil {
						return fmt.Errorf("failed to write rollback plan: %w", err)
					}
				}

				if output != "" {
					if output != "yaml" && output != "json" {
						return fmt.Errorf("invalid output format: %s, allowed values: yaml, json", output)
					}
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
						"deleted": diff.Delete,
					}
					return utils.PrintStructured("apply", applyResult, output, "")
				}
				return nil
			}

			if output != "" {
				if output != "yaml" && output != "json" {
					return fmt.Errorf("invalid output format: %s, allowed values: yaml, json", output)
				}
				return utils.PrintStructured("diff", diff, output, "")
			}
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

type MergeCheckDiff = models.RepoDiff

var mergeCheckOps = utils.RepoItemOps[models.ScriptRunnerMergeCheck, int64]{
	GetItems: func(r models.ExtendedRepository) []models.ScriptRunnerMergeCheck {
		if r.MergeChecks == nil {
			return nil
		}
		return *r.MergeChecks
	},
	SetItems: func(r *models.ExtendedRepository, items []models.ScriptRunnerMergeCheck) { r.MergeChecks = &items },
	GetID: func(it models.ScriptRunnerMergeCheck) (int64, bool) {
		return bitbucket.MergeCheckID(it)
	},
	Equal: bitbucket.AreMergeChecksEqual,
}

func generateMergeCheckDiff(src, tgt []models.ExtendedRepository) (*MergeCheckDiff, error) {
	return utils.GenerateRepoDiff(src, tgt, mergeCheckOps)
}

// findSourceCheckByID looks up the merge check with the given id in the source repo
func findSourceCheckByID(source []models.ExtendedRepository, projectKey, slug string, id int64) (models.ScriptRunnerMergeCheck, bool) {
	for _, r := range source {
		if r.ProjectKey != projectKey || r.RepositorySlug != slug {
			continue
		}
		if r.MergeChecks == nil {
			continue
		}
		for _, mc := range *r.MergeChecks {
			if mcID, ok := bitbucket.MergeCheckID(mc); ok && mcID == id {
				return mc, true
			}
		}
	}
	return nil, false
}

// mergeUpdateRepos merges diff update items with the current state from source.
func mergeUpdateRepos(source []models.ExtendedRepository, update []models.ExtendedRepository) []models.ExtendedRepository {
	mergedRepos := []models.ExtendedRepository{}
	for _, r := range update {
		if r.MergeChecks == nil || len(*r.MergeChecks) == 0 {
			continue
		}
		merged := []models.ScriptRunnerMergeCheck{}
		for _, tc := range *r.MergeChecks {
			id, ok := bitbucket.MergeCheckID(tc)
			if !ok {
				merged = append(merged, tc)
				continue
			}
			current, found := findSourceCheckByID(source, r.ProjectKey, r.RepositorySlug, id)
			if found {
				merged = append(merged, bitbucket.MergeUpdateMergeChecks(current, tc))
			} else {
				merged = append(merged, tc)
			}
		}
		rr := models.ExtendedRepository{ProjectKey: r.ProjectKey, RepositorySlug: r.RepositorySlug}
		rr.MergeChecks = &merged
		mergedRepos = append(mergedRepos, rr)
	}
	return mergedRepos
}
