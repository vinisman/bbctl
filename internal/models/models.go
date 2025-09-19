package models

import "github.com/vinisman/bitbucket-sdk-go/openapi"

type WebhookResponse struct {
	Size       int                   `json:"size,omitempty" yaml:"size,omitempty"`
	Limit      int                   `json:"limit,omitempty" yaml:"limit,omitempty"`
	IsLastPage bool                  `json:"isLastPage,omitempty" yaml:"isLastPage,omitempty"`
	Values     []openapi.RestWebhook `json:"values,omitempty" yaml:"values,omitempty"`
	Start      int                   `json:"start,omitempty" yaml:"start,omitempty"`
}

// Extended repository struct
type ExtendedRepository struct {
	ProjectKey     string                                `json:"projectKey,omitempty" yaml:"projectKey,omitempty"`
	RepositorySlug string                                `json:"repositorySlug,omitempty" yaml:"repositorySlug,omitempty"`
	DefaultBranch  string                                `json:"defaultBranch,omitempty" yaml:"defaultBranch,omitempty"`
	RestRepository *openapi.RestRepository               `json:"restRepository,omitempty" yaml:"restRepository,omitempty"`
	Webhooks       *[]openapi.RestWebhook                `json:"webhooks,omitempty" yaml:"webhooks,omitempty"`
	Manifest       *map[string]interface{}               `json:"manifest,omitempty" yaml:"manifest,omitempty"`
	RequiredBuilds *[]openapi.RestRequiredBuildCondition `json:"requiredBuilds,omitempty" yaml:"requiredBuilds,omitempty"`
}

type RepositoryOptions struct {
	Repository     bool
	Webhooks       bool
	DefaultBranch  bool
	Manifest       bool
	ManifestPath   *string
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
