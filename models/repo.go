package models

type RepoEntity struct {
	Id            uint   `json:"id"`
	Name          string `json:"name" yaml:"name"`
	Slug          string `json:"slug,omitempty"`
	Description   string `json:"description,omitempty" yaml:"description,omitempty"`
	State         string `json:"state,omitempty"`
	Public        bool   `json:"public,omitempty" yaml:"public,omitempty"`
	Archived      bool   `json:"archived,omitempty" yaml:"archived,omitempty"`
	Forkable      bool   `json:"forkable,omitempty" yaml:"forkable,omitempty"`
	DefaultBranch string `json:"-" yaml:"defaultBranch,omitempty"`
}

type RepoListInput struct {
	Repositories []RepoEntity `yaml:"repositories"`
}

type RepoListResponse struct {
	IsLastPage    bool         `json:"isLastPage"`
	NextPageStart int          `json:"nextPageStart"`
	Values        []RepoEntity `json:"values"`
}
