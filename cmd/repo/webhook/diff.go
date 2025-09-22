package webhook

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
	"github.com/vinisman/bbctl/internal/bitbucket"
	"github.com/vinisman/bbctl/internal/models"
	"github.com/vinisman/bbctl/utils"
	openapi "github.com/vinisman/bitbucket-sdk-go/openapi"
)

func DiffWebHookCmd() *cobra.Command {
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
		Short: "Compare webhooks between two files and generate/apply diff",
		Long: `Compare webhooks between two YAML/JSON files and generate a diff with three sections:
 - create: items present in TARGET without id, and items with ids that are in TARGET but not in SOURCE
 - update: items with the same id present in both files, but with different fields
 - delete: items with ids present in SOURCE but not in TARGET

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
					if err := client.DeleteWebhooks(plan.Delete); err != nil {
						return fmt.Errorf("rollback delete failed: %w", err)
					}
				}
				if len(plan.Update) > 0 {
					if _, err := client.UpdateWebhooks(plan.Update); err != nil {
						return fmt.Errorf("rollback update failed: %w", err)
					}
				}
				if len(plan.Create) > 0 {
					if _, err := client.CreateWebhooks(plan.Create); err != nil {
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

			diff, err := generateWebhookDiff(parsedSource.Repositories, parsedTarget.Repositories)
			if err != nil {
				return fmt.Errorf("failed to generate diff: %w", err)
			}

			if forceUpdate {
				diff.Update = utils.ForceUpdateBySourceIDs(parsedSource.Repositories, parsedTarget.Repositories, utils.RepoItemOps[openapi.RestWebhook, int32]{
					GetItems: func(r models.ExtendedRepository) []openapi.RestWebhook {
						if r.Webhooks == nil {
							return nil
						}
						return *r.Webhooks
					},
					SetItems: func(r *models.ExtendedRepository, items []openapi.RestWebhook) { r.Webhooks = &items },
					GetID: func(it openapi.RestWebhook) (int32, bool) {
						if it.Id == nil {
							return 0, false
						}
						return *it.Id, true
					},
					Equal: bitbucket.AreWebhooksEqual,
				})
			}

			if apply {
				client, err := bitbucket.NewClient(context.Background())
				if err != nil {
					return err
				}

				if len(diff.Delete) > 0 {
					if err := client.DeleteWebhooks(diff.Delete); err != nil {
						return fmt.Errorf("apply delete failed: %w", err)
					}
				}

				var updatedRepos []models.ExtendedRepository
				if len(diff.Update) > 0 {
					updatedRepos, err = client.UpdateWebhooks(diff.Update)
					if err != nil {
						return fmt.Errorf("apply update failed: %w", err)
					}
				}

				var createdRepos []models.ExtendedRepository
				if len(diff.Create) > 0 {
					createdRepos, err = client.CreateWebhooks(diff.Create)
					if err != nil {
						return fmt.Errorf("apply create failed: %w", err)
					}
				}

				if applyRollbackOut != "" {
					rollbackPlan := utils.BuildRollbackPlan(parsedSource.Repositories, *diff, updatedRepos, createdRepos, utils.RepoItemOps[openapi.RestWebhook, int32]{
						GetItems: func(r models.ExtendedRepository) []openapi.RestWebhook {
							if r.Webhooks == nil {
								return nil
							}
							return *r.Webhooks
						},
						SetItems: func(r *models.ExtendedRepository, items []openapi.RestWebhook) { r.Webhooks = &items },
						GetID: func(it openapi.RestWebhook) (int32, bool) {
							if it.Id == nil {
								return 0, false
							}
							return *it.Id, true
						},
						Equal: bitbucket.AreWebhooksEqual,
					})
					if err := utils.WriteRollbackPlan(applyRollbackOut, output, rollbackPlan); err != nil {
						return fmt.Errorf("failed to write rollback plan: %w", err)
					}
				}

				if output != "" {
					if output != "yaml" && output != "json" {
						return fmt.Errorf("invalid output format: %s, allowed values: yaml, json", output)
					}
					// write apply-result-out file if requested
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

type WebhookDiff = models.RepoDiff

func generateWebhookDiff(src, tgt []models.ExtendedRepository) (*WebhookDiff, error) {
	return utils.GenerateRepoDiff(src, tgt, utils.RepoItemOps[openapi.RestWebhook, int32]{
		GetItems: func(r models.ExtendedRepository) []openapi.RestWebhook {
			if r.Webhooks == nil {
				return nil
			}
			return *r.Webhooks
		},
		SetItems: func(r *models.ExtendedRepository, items []openapi.RestWebhook) { r.Webhooks = &items },
		GetID: func(it openapi.RestWebhook) (int32, bool) {
			if it.Id == nil {
				return 0, false
			}
			return *it.Id, true
		},
		Equal: bitbucket.AreWebhooksEqual,
	})
}
