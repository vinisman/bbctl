package models

// ScriptRunnerMergeCheck is a ScriptRunner canned merge check.
// The field set depends on the canned-script type, hence the free-form model.
// Diff matches on the "id" field; comparison uses target keys (subset).
type ScriptRunnerMergeCheck map[string]any

// DefaultTask is a repository default task (Bitbucket REST /rest/default-tasks/latest).
type DefaultTask map[string]any

// DefaultTasksResponse is the response of GET /rest/default-tasks/latest/projects/{pk}/repos/{slug}/tasks
type DefaultTasksResponse struct {
	Size       int          `json:"size,omitempty" yaml:"size,omitempty"`
	Limit      int          `json:"limit,omitempty" yaml:"limit,omitempty"`
	IsLastPage bool         `json:"isLastPage,omitempty" yaml:"isLastPage,omitempty"`
	Start      int          `json:"start,omitempty" yaml:"start,omitempty"`
	Values     []DefaultTask `json:"values,omitempty" yaml:"values,omitempty"`
}
