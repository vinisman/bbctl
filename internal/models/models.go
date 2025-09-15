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
