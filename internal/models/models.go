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

type RepositoryYaml struct {
	Repositories []ExtendedRepository `yaml:"repositories"`
}

type ProjectYaml struct {
	Projects []openapi.RestProject `yaml:"projects"`
}

type ProjectList struct {
	Projects []string `yaml:"projects"`
}
