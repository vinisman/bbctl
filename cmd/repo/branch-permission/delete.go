package branchpermission

import (
	"context"
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"github.com/vinisman/bbctl/internal/bitbucket"
	"github.com/vinisman/bbctl/internal/models"
	"github.com/vinisman/bbctl/utils"
	openapi "github.com/vinisman/bitbucket-sdk-go/openapi"
)

// DeleteBranchPermissionCmd returns a cobra command to delete branch permissions from a YAML file
func DeleteBranchPermissionCmd() *cobra.Command {
	var input string
	var repositorySlug string
	var ids string

	cmd := &cobra.Command{
		Use:   "delete",
		Short: "Delete branch permissions",
		Long: `Delete branch permissions by ID.

Can delete from command line using --ids flag or from a YAML file using --input.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := bitbucket.NewClient(context.Background())
			if err != nil {
				return err
			}

			var repositories []models.ExtendedRepository

			if input != "" {
				var parsed models.RepositoryYaml
				if err := utils.ParseFile(input, &parsed); err != nil {
					return err
				}
				repositories = parsed.Repositories
			} else {
				if repositorySlug == "" || ids == "" {
					return fmt.Errorf("please specify --repositorySlug and --ids, or use --input")
				}

				repositories = []models.ExtendedRepository{}
				items := strings.Split(repositorySlug, ",")
				idList := strings.Split(ids, ",")

				for _, item := range items {
					item = strings.TrimSpace(item)
					parts := strings.SplitN(item, "/", 2)
					if len(parts) != 2 || parts[1] == "" {
						client.Logger.Error(fmt.Sprintf("invalid repository identifier format: %s", item))
						return fmt.Errorf("invalid repository identifier format: %s", item)
					}

					permissions := []openapi.RestRefRestriction{}
					for _, idStr := range idList {
						idStr = strings.TrimSpace(idStr)
						var id int32
						fmt.Sscanf(idStr, "%d", &id)
						permissions = append(permissions, openapi.RestRefRestriction{
							Id: &id,
						})
					}

					repositories = append(repositories, models.ExtendedRepository{
						ProjectKey:        parts[0],
						RepositorySlug:    parts[1],
						BranchPermissions: &permissions,
					})
				}
			}

			hasPermissions := false
			for _, repo := range repositories {
				if repo.BranchPermissions != nil && len(*repo.BranchPermissions) > 0 {
					hasPermissions = true
					break
				}
			}

			if !hasPermissions {
				return fmt.Errorf("no branch permissions defined for deletion")
			}

			err = client.DeleteBranchPermissions(repositories)
			if err != nil {
				client.Logger.Error(err.Error())
			}

			return nil
		},
	}

	cmd.Flags().StringVarP(&input, "input", "i", "", `Path to YAML or JSON file or '-' to read from stdin
Example:
repositories:
  - projectKey: DEV
    repositorySlug: my-repo
    branchPermissions:
      - id: 1
      - id: 2
`)
	cmd.Flags().StringVarP(&repositorySlug, "repositorySlug", "s", "", "Repository identifiers in format <projectKey>/<repositorySlug>, multiple repositories can be comma-separated")
	cmd.Flags().StringVarP(&ids, "ids", "", "", "Comma-separated list of branch permission IDs to delete")

	return cmd
}
