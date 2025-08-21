package models

import "github.com/vinisman/bitbucket-sdk-go/openapi"

type WebhookResponse struct {
	Size       int                   `json:"size"`
	Limit      int                   `json:"limit"`
	IsLastPage bool                  `json:"isLastPage"`
	Values     []openapi.RestWebhook `json:"values"`
	Start      int                   `json:"start"`
}

// Extended repository struct
type ExtendedRepository struct {
	ProjectKey     string                               `yaml:"projectKey,omitempty"`
	RepositorySlug string                               `yaml:"repositorySlug,omitempty"`
	RestRepository openapi.RestRepository               `yaml:"repository,omitempty"`
	Webhooks       []openapi.RestWebhook                `yaml:"webhooks,omitempty"`
	Manifest       map[string]interface{}               `yaml:"manifest,omitempty"`
	RequiredBuilds []openapi.RestRequiredBuildCondition `yaml:"requiredBuilds,omitempty"`
}

type RepositoryOptions struct {
	Repository     bool
	Webhooks       bool
	DefaultBranch  bool
	Manifest       bool
	ManifestPath   *string
	RequiredBuilds bool
}

// RepoRef represents a repository reference with project key and slug
type RepoRef struct {
	Project string `yaml:"project"`
	Slug    string `yaml:"slug"`
}

type RepositoryYaml struct {
	Repositories []ExtendedRepository `yaml:"repositories"`
}
