package models

type RepoWithBuildKeys struct {
	Name      string   `yaml:"name" validate:"required"`
	Branch    string   `yaml:"branch" validate:"required"`
	BuildKeys []string `yaml:"buildKeys" validate:"required"`
}

type RepoBuildFile struct {
	Repositories []RepoWithBuildKeys `yaml:"repositories" validate:"required"`
}
