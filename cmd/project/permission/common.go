package permission

import (
	"fmt"

	"github.com/vinisman/bbctl/internal/models"
	"github.com/vinisman/bbctl/utils"
)

// parseProjectKeys builds a project list from comma-separated project keys.
func parseProjectKeys(projectKeys string) []models.ProjectPermissions {
	keys := utils.ParseColumns(projectKeys)
	projects := make([]models.ProjectPermissions, 0, len(keys))
	for _, key := range keys {
		projects = append(projects, models.ProjectPermissions{ProjectKey: key})
	}
	return projects
}

// buildProjects attaches the same permission to every user and group for every
// project given on the command line.
func buildProjects(projectKeys, users, groups, permission string) []models.ProjectPermissions {
	projects := parseProjectKeys(projectKeys)
	for i := range projects {
		for _, u := range utils.ParseColumns(users) {
			projects[i].Users = append(projects[i].Users, models.PermissionEntry{Name: u, Permission: permission})
		}
		for _, g := range utils.ParseColumns(groups) {
			projects[i].Groups = append(projects[i].Groups, models.PermissionEntry{Name: g, Permission: permission})
		}
	}
	return projects
}

// validateProjects checks the input and normalizes permission levels.
// When requirePermission is false (revoke), permission levels are not required.
func validateProjects(projects []models.ProjectPermissions, requirePermission bool) error {
	if len(projects) == 0 {
		return fmt.Errorf("no projects specified")
	}

	for i := range projects {
		p := &projects[i]
		if p.ProjectKey == "" {
			return fmt.Errorf("projects[%d]: projectKey is required", i)
		}
		if len(p.Users) == 0 && len(p.Groups) == 0 {
			return fmt.Errorf("project %s: no users or groups specified", p.ProjectKey)
		}

		for j := range p.Users {
			if p.Users[j].Name == "" {
				return fmt.Errorf("project %s: users[%d]: name is required", p.ProjectKey, j)
			}
			if requirePermission {
				perm, err := models.NormalizePermission(p.Users[j].Permission, models.ProjectPermissionLevels)
				if err != nil {
					return fmt.Errorf("project %s: user %s: %w", p.ProjectKey, p.Users[j].Name, err)
				}
				p.Users[j].Permission = perm
			}
		}

		for j := range p.Groups {
			if p.Groups[j].Name == "" {
				return fmt.Errorf("project %s: groups[%d]: name is required", p.ProjectKey, j)
			}
			if requirePermission {
				perm, err := models.NormalizePermission(p.Groups[j].Permission, models.ProjectPermissionLevels)
				if err != nil {
					return fmt.Errorf("project %s: group %s: %w", p.ProjectKey, p.Groups[j].Name, err)
				}
				p.Groups[j].Permission = perm
			}
		}
	}

	return nil
}

// projectPermissionRows flattens project permissions for output.
func projectPermissionRows(projects []models.ProjectPermissions) []models.PermissionRow {
	var rows []models.PermissionRow
	for _, p := range projects {
		for _, u := range p.Users {
			rows = append(rows, models.PermissionRow{
				ProjectKey: p.ProjectKey,
				Type:       "user",
				Name:       u.Name,
				Permission: u.Permission,
			})
		}
		for _, g := range p.Groups {
			rows = append(rows, models.PermissionRow{
				ProjectKey: p.ProjectKey,
				Type:       "group",
				Name:       g.Name,
				Permission: g.Permission,
			})
		}
	}
	return rows
}
