package models

import (
	openapi "github.com/vinisman/bitbucket-sdk-go/openapi"
	workzone "github.com/vinisman/workzone-sdk-go/client"
)

type WebhookResponse struct {
	Size       int                   `json:"size,omitempty" yaml:"size,omitempty"`
	Limit      int                   `json:"limit,omitempty" yaml:"limit,omitempty"`
	IsLastPage bool                  `json:"isLastPage,omitempty" yaml:"isLastPage,omitempty"`
	Values     []openapi.RestWebhook `json:"values,omitempty" yaml:"values,omitempty"`
	Start      int                   `json:"start,omitempty" yaml:"start,omitempty"`
}

// Extended repository struct
type ExtendedRepository struct {
	ProjectKey         string                                `json:"projectKey,omitempty" yaml:"projectKey,omitempty"`
	RepositorySlug     string                                `json:"repositorySlug,omitempty" yaml:"repositorySlug,omitempty"`
	DefaultBranch      string                                `json:"defaultBranch,omitempty" yaml:"defaultBranch,omitempty"`
	RestRepository     *openapi.RestRepository               `json:"restRepository,omitempty" yaml:"restRepository,omitempty"`
	Webhooks           *[]openapi.RestWebhook                `json:"webhooks,omitempty" yaml:"webhooks,omitempty"`
	BranchPermissions  *[]openapi.RestRefRestriction         `json:"branchPermissions,omitempty" yaml:"branchPermissions,omitempty"`
	Manifest           *map[string]any                       `json:"manifest,omitempty" yaml:"manifest,omitempty"`
	ConfigFiles        *map[string]any                       `json:"configFiles,omitempty" yaml:"configFiles,omitempty"`
	RequiredBuilds     *[]openapi.RestRequiredBuildCondition `json:"requiredBuilds,omitempty" yaml:"requiredBuilds,omitempty"`
	ReviewerGroups     *[]openapi.RestReviewerGroup          `json:"reviewerGroups,omitempty" yaml:"reviewerGroups,omitempty"`
	Workzone           *WorkzoneData                         `json:"workzone,omitempty" yaml:"workzone,omitempty"`
}

// WorkzoneData groups Workzone-related sections for a repository
type WorkzoneData struct {
	WorkflowProperties *workzone.WorkflowProperties `json:"workflowProperties,omitempty" yaml:"workflowProperties,omitempty"`
	// Future sections: Reviewers, AutoMergers, Signatures, GlobalConfig, etc.
	Reviewers     []workzone.RestBranchReviewers     `json:"reviewers,omitempty" yaml:"reviewers,omitempty"`
	Signapprovers []workzone.RestBranchSignapprovers `json:"signapprovers,omitempty" yaml:"signapprovers,omitempty"`
	Mergerules    []workzone.RestBranchAutoMergers   `json:"mergerules,omitempty" yaml:"mergerules,omitempty"`
}

type RepositoryOptions struct {
	Repository     bool
	Webhooks       bool
	DefaultBranch  bool
	Manifest       bool
	ManifestPath   *string
	ConfigFiles    bool
	ConfigFileMap  map[string]string
	RequiredBuilds bool
}

type RepositoryYaml struct {
	Repositories []ExtendedRepository `json:"repositories,omitempty" yaml:"repositories,omitempty"`
}

type ProjectYaml struct {
	Projects []openapi.RestProject `json:"projects,omitempty" yaml:"projects,omitempty"`
}

// User represents a user for YAML parsing (with string fields)
type User struct {
	Name         string `json:"name" yaml:"name"`
	DisplayName  string `json:"displayName" yaml:"displayName"`
	EmailAddress string `json:"emailAddress" yaml:"emailAddress"`
}

type UserYaml struct {
	Users []User `json:"users,omitempty" yaml:"users,omitempty"`
}

// Group represents a group for YAML parsing (with string fields)
type Group struct {
	Name string `json:"name" yaml:"name"`
}

type GroupYaml struct {
	Groups []Group `json:"groups,omitempty" yaml:"groups,omitempty"`
}

// RollbackPlan represents a set of repository-level changes that can be
// executed (apply) or reversed (rollback). It is generic for different
// domains (required-builds, webhooks) since the payload lives inside
// ExtendedRepository fields (RequiredBuilds or Webhooks).
type RollbackPlan struct {
	Delete []ExtendedRepository `json:"delete" yaml:"delete"`
	Update []ExtendedRepository `json:"update" yaml:"update"`
	Create []ExtendedRepository `json:"create" yaml:"create"`
}

// RepoDiff is a generic diff container for repository-scoped items.
// The concrete items (required-builds, webhooks) live inside ExtendedRepository fields.
type RepoDiff struct {
	Create []ExtendedRepository `json:"create" yaml:"create"`
	Update []ExtendedRepository `json:"update" yaml:"update"`
	Delete []ExtendedRepository `json:"delete" yaml:"delete"`
}

// RepositoryYamlInput is the input format for create/update branch permissions
// Uses RestRefRestrictionCreate which accepts users as strings
type RepositoryYamlInput struct {
	Repositories []struct {
		ProjectKey        string                        `json:"projectKey" yaml:"projectKey"`
		RepositorySlug    string                        `json:"repositorySlug" yaml:"repositorySlug"`
		BranchPermissions *[]openapi.RestRefRestrictionCreate `json:"branchPermissions,omitempty" yaml:"branchPermissions,omitempty"`
	} `json:"repositories" yaml:"repositories"`
}

// ToRepositoryYaml converts input to internal format for API operations
func (r *RepositoryYamlInput) ToRepositoryYaml() RepositoryYaml {
	result := RepositoryYaml{
		Repositories: make([]ExtendedRepository, 0, len(r.Repositories)),
	}
	for _, repo := range r.Repositories {
		extRepo := ExtendedRepository{
			ProjectKey:     repo.ProjectKey,
			RepositorySlug: repo.RepositorySlug,
		}
		if repo.BranchPermissions != nil {
			// Convert RestRefRestrictionCreate to RestRefRestriction for storage
			perms := make([]openapi.RestRefRestriction, 0, len(*repo.BranchPermissions))
			for _, bp := range *repo.BranchPermissions {
				perm := openapi.RestRefRestriction{
					Id:         bp.Id,
					Type:       bp.Type,
					Matcher:    bp.Matcher,
					Scope:      bp.Scope,
					Groups:     bp.Groups,
					AccessKeys: bp.AccessKeys,
				}
				// Convert user strings to objects
				for _, userName := range bp.Users {
					perm.Users = append(perm.Users, openapi.RestApplicationUser{
						Name: &userName,
					})
				}
				perms = append(perms, perm)
			}
			extRepo.BranchPermissions = &perms
		}
		result.Repositories = append(result.Repositories, extRepo)
	}
	return result
}
