package defaulttask

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
	"github.com/vinisman/bbctl/internal/bitbucket"
	"github.com/vinisman/bbctl/internal/models"
	"github.com/vinisman/bbctl/utils"
)

func DiffDefaultTaskCmd() *cobra.Command {
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
		Short: "Compare default tasks between two files and generate/apply diff",
		Long: `Compare default tasks between two YAML/JSON files and generate a diff with three sections:
 - create: items present in TARGET without id, and items with ids that are in TARGET but not in SOURCE
 - update: items with the same id present in both files, but with different fields
 - delete: items with ids present in SOURCE but not in TARGET

Default tasks are compared by target keys only (target subset of current); the id field is ignored.
Default tasks do not support update: on --apply, an update is executed as delete of the existing
task followed by create of the target task.

Options:
 - --apply: execute the diff against Bitbucket (delete, then update as delete+create, then create)
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
					if err := client.DeleteDefaultTasks(plan.Delete); err != nil {
						return fmt.Errorf("rollback delete failed: %w", err)
					}
				}
				if len(plan.Update) > 0 {
					// default tasks do not support update: delete by id + create without id
					if err := client.DeleteDefaultTasks(plan.Update); err != nil {
						return fmt.Errorf("rollback update (delete) failed: %w", err)
					}
					recreated := stripTaskIDs(plan.Update)
					if len(recreated) > 0 {
						if _, err := client.CreateDefaultTasks(recreated); err != nil {
							return fmt.Errorf("rollback update (create) failed: %w", err)
						}
					}
				}
				if len(plan.Create) > 0 {
					if _, err := client.CreateDefaultTasks(plan.Create); err != nil {
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

			diff, err := generateDefaultTaskDiff(parsedSource.Repositories, parsedTarget.Repositories)
			if err != nil {
				return fmt.Errorf("failed to generate diff: %w", err)
			}

			if forceUpdate {
				diff.Update = utils.ForceUpdateBySourceIDs(parsedSource.Repositories, parsedTarget.Repositories, defaultTaskOps)
			}

			if apply {
				client, err := bitbucket.NewClient(context.Background())
				if err != nil {
					return err
				}

				if len(diff.Delete) > 0 {
					if err := client.DeleteDefaultTasks(diff.Delete); err != nil {
						return fmt.Errorf("apply delete failed: %w", err)
					}
				}

				// update: default tasks do not support update — delete the old one + create the new one
				var updatedRepos []models.ExtendedRepository
				if len(diff.Update) > 0 {
					if err := client.DeleteDefaultTasks(diff.Update); err != nil {
						return fmt.Errorf("apply update (delete old task) failed: %w", err)
					}
					recreated := stripTaskIDs(diff.Update)
					if len(recreated) > 0 {
						updatedRepos, err = client.CreateDefaultTasks(recreated)
						if err != nil {
							return fmt.Errorf("apply update (create new task) failed: %w", err)
						}
					}
				}

				var createdRepos []models.ExtendedRepository
				if len(diff.Create) > 0 {
					createdRepos, err = client.CreateDefaultTasks(diff.Create)
					if err != nil {
						return fmt.Errorf("apply create failed: %w", err)
					}
				}

				if applyRollbackOut != "" {
					rollbackPlan := utils.BuildRollbackPlan(parsedSource.Repositories, *diff, updatedRepos, createdRepos, defaultTaskOps)
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
	cmd.Flags().BoolVarP(&apply, "apply", "a", false, "Apply the diff to Bitbucket: delete, then update as delete+create, then create")
	cmd.Flags().BoolVar(&forceUpdate, "force-update", false, "Force update section to include ALL target items whose id exists in source")
	cmd.Flags().StringVar(&applyResultOut, "apply-result-out", "", "After --apply: FILE path to save created+updated repos; format controlled by -o (json/yaml)")
	cmd.Flags().StringVar(&applyRollbackOut, "apply-rollback-out", "", "Write rollback plan to file after successful --apply (json or yaml)")
	cmd.Flags().StringVar(&rollbackFile, "rollback", "", "Execute rollback plan from file (reverses a previous apply)")
	cmd.Flags().BoolVarP(&quiet, "quiet", "q", false, "Suppress printing rollback plan to stdout during --rollback")

	return cmd
}

type DefaultTaskDiff = models.RepoDiff

var defaultTaskOps = utils.RepoItemOps[models.DefaultTask, int64]{
	GetItems: func(r models.ExtendedRepository) []models.DefaultTask {
		if r.DefaultTasks == nil {
			return nil
		}
		return *r.DefaultTasks
	},
	SetItems: func(r *models.ExtendedRepository, items []models.DefaultTask) { r.DefaultTasks = &items },
	GetID: func(it models.DefaultTask) (int64, bool) {
		return bitbucket.DefaultTaskID(it)
	},
	Equal: bitbucket.AreDefaultTasksEqual,
}

func generateDefaultTaskDiff(src, tgt []models.ExtendedRepository) (*DefaultTaskDiff, error) {
	return utils.GenerateRepoDiff(src, tgt, defaultTaskOps)
}

// stripTaskIDs removes task ids: a create-POST must not carry the old id.
func stripTaskIDs(repos []models.ExtendedRepository) []models.ExtendedRepository {
	out := []models.ExtendedRepository{}
	for _, r := range repos {
		if r.DefaultTasks == nil {
			continue
		}
		clean := []models.DefaultTask{}
		for _, t := range *r.DefaultTasks {
			cp := models.DefaultTask{}
			for k, v := range t {
				if k == "id" {
					continue
				}
				cp[k] = v
			}
			clean = append(clean, cp)
		}
		if len(clean) > 0 {
			rr := models.ExtendedRepository{ProjectKey: r.ProjectKey, RepositorySlug: r.RepositorySlug}
			rr.DefaultTasks = &clean
			out = append(out, rr)
		}
	}
	return out
}
