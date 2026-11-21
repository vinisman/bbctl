package permission

import (
	"fmt"
	"strings"

	"github.com/vinisman/bbctl/internal/models"
	"github.com/vinisman/bbctl/utils"
)

// parseRepositories builds a repository list from comma-separated
// <projectKey>/<repositorySlug> identifiers.
func parseRepositories(repositorySlugs string) ([]models.RepositoryPermissions, error) {
	items := utils.ParseColumns(repositorySlugs)
	repos := make([]models.RepositoryPermissions, 0, len(items))
	for _, item := range items {
		parts := strings.SplitN(item, "/", 2)
		if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
			return nil, fmt.Errorf("invalid repository identifier format: %s (expected <projectKey>/<repositorySlug>)", item)
		}
		repos = append(repos, models.RepositoryPermissions{
			ProjectKey:     parts[0],
			RepositorySlug: parts[1],
		})
	}
	return repos, nil
}

// buildRepositories attaches the same permission to every user and group for
// every repository given on the command line.
func buildRepositories(repositorySlugs, users, groups, permission string) ([]models.RepositoryPermissions, error) {
	repos, err := parseRepositories(repositorySlugs)
	if err != nil {
		return nil, err
	}
	for i := range repos {
		for _, u := range utils.ParseColumns(users) {
			repos[i].Users = append(repos[i].Users, models.PermissionEntry{Name: u, Permission: permission})
		}
		for _, g := range utils.ParseColumns(groups) {
			repos[i].Groups = append(repos[i].Groups, models.PermissionEntry{Name: g, Permission: permission})
		}
	}
	return repos, nil
}

// validateRepositories checks the input and normalizes permission levels.
// When requirePermission is false (revoke), permission levels are not required.
func validateRepositories(repos []models.RepositoryPermissions, requirePermission bool) error {
	if len(repos) == 0 {
		return fmt.Errorf("no repositories specified")
	}

	for i := range repos {
		r := &repos[i]
		if r.ProjectKey == "" || r.RepositorySlug == "" {
			return fmt.Errorf("repositories[%d]: projectKey and repositorySlug are required", i)
		}
		if len(r.Users) == 0 && len(r.Groups) == 0 {
			return fmt.Errorf("%s/%s: no users or groups specified", r.ProjectKey, r.RepositorySlug)
		}

		for j := range r.Users {
			if r.Users[j].Name == "" {
				return fmt.Errorf("%s/%s: users[%d]: name is required", r.ProjectKey, r.RepositorySlug, j)
			}
			if requirePermission {
				perm, err := models.NormalizePermission(r.Users[j].Permission, models.RepositoryPermissionLevels)
				if err != nil {
					return fmt.Errorf("%s/%s: user %s: %w", r.ProjectKey, r.RepositorySlug, r.Users[j].Name, err)
				}
				r.Users[j].Permission = perm
			}
		}

		for j := range r.Groups {
			if r.Groups[j].Name == "" {
				return fmt.Errorf("%s/%s: groups[%d]: name is required", r.ProjectKey, r.RepositorySlug, j)
			}
			if requirePermission {
				perm, err := models.NormalizePermission(r.Groups[j].Permission, models.RepositoryPermissionLevels)
				if err != nil {
					return fmt.Errorf("%s/%s: group %s: %w", r.ProjectKey, r.RepositorySlug, r.Groups[j].Name, err)
				}
				r.Groups[j].Permission = perm
			}
		}
	}

	return nil
}

// repositoryPermissionRows flattens repository permissions for output.
func repositoryPermissionRows(repos []models.RepositoryPermissions) []models.PermissionRow {
	var rows []models.PermissionRow
	for _, r := range repos {
		for _, u := range r.Users {
			rows = append(rows, models.PermissionRow{
				ProjectKey:     r.ProjectKey,
				RepositorySlug: r.RepositorySlug,
				Type:           "user",
				Name:           u.Name,
				Permission:     u.Permission,
			})
		}
		for _, g := range r.Groups {
			rows = append(rows, models.PermissionRow{
				ProjectKey:     r.ProjectKey,
				RepositorySlug: r.RepositorySlug,
				Type:           "group",
				Name:           g.Name,
				Permission:     g.Permission,
			})
		}
	}
	return rows
}
