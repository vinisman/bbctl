# bbctl

**bbctl** is a CLI tool for managing repositories and automation in **Bitbucket Server / Data Center** environments.  
It provides streamlined support for creating, deleting, updating, and retrieving information about projects and repositories.

## ✨ Features

- Manage multiple repositories and multiple projects
- Retrieve additional repository information using a manifest file from the root of the repository  
- Parallel processing for high-performance bulk operations
- YAML input/output for full GitOps compatibility
- Easy configuration via `.env` file

## Configuration
Add a `.env` properties file as shown below, or provide configuration via command-line flags.  
For details see --help
```
BITBUCKET_BASE_URL=https://bitbucket.local/rest
BITBUCKET_TOKEN=<token>
BITBUCKET_PAGE_SIZE=50
```


## Supported operations

### For projects
- **Create** new projects
- **Delete** existing projects
- **Update** project information (such as name and description)
- **Retrieve basic info** about projects

### For repositories
- **Create** new repositories
- **Delete** existing repositories
- **Create forks** of repositories
- **Update** repository information (including moving repositories between projects)
- **Retrieve basic info** about repositories
- **Retrieve detailed info** for repositories, including:
  - Webhooks
  - Required builds
  - Manifest file information (from the root of the repository)
  - Default branch

## Usage examples

List projects in plain format
```
$ bbctl project get -k project_1
Id  Name       Key        Description
53  Project_1  PROJECT_1  Description for project_11
```

List projects in yaml format
```
$ bbctl project get -k project_1 -o yaml
projects:
    - avatar: null
      avatarurl: null
      description: Description for project_11
      id: 53
      key: PROJECT_1
      links:
        self:
            - href: https://bitbucket.local/projects/PROJECT_1
      name: Project_1
      public: false
      scope: null
      type: NORMAL
```

Create project from CLI
```
$ bbctl project create -k demo_project --name demoProject
time=2025-08-21T14:46:02.342+03:00 level=INFO msg="Created project" key=DEMO_PROJECT
```

Bulk create projects from yaml file
```
$ bbctl project create -i examples/projects/create.yaml
time=2025-08-21T14:58:26.786+03:00 level=INFO msg="Created project" key=D_PROJECT_2
time=2025-08-21T14:58:26.792+03:00 level=INFO msg="Created project" key=D_PROJECT_1
time=2025-08-21T14:58:26.792+03:00 level=INFO msg="Created project" key=D_PROJECT_3
```

List repositories in plain format
```
$ bbctl repo get -k PROJECT_1 --columns id,name,slug,project
id  name    slug    project
46  repo1   repo1   Project_1
54  repo10  repo10  Project_1
51  repo2   repo2   Project_1
58  repo3   repo3   Project_1
57  repo4   repo4   Project_1
53  repo5   repo5   Project_1
55  repo6   repo6   Project_1
56  repo7   repo7   Project_1
52  repo8   repo8   Project_1
50  repo9   repo9   Project_1
```


Get repository in yaml format
```
$ bbctl repo get -k PROJECT_1 -s repo1 -o yaml
repositories:
    - projectKey: PROJECT_1
      repositorySlug: repo1
      repository:
        archived: false
        defaultbranch: null
        description: Description for repo1
        forkable: true
        hierarchyid: 7f5b2de61af87cf7135c
        id: 46
        links:
            clone:
                - href: https://bitbucket.local/scm/project_1/repo1.git
                  name: http
            self:
                - href: https://bitbucket.local/projects/PROJECT_1/repos/repo1/browse
        name: repo1
        origin: null
        partition: null
        project:
            avatar: null
            avatarurl: null
            description: Description for project_11
            id: 53
            key: PROJECT_1
            links:
                self:
                    - href: https://bitbucket.local/projects/PROJECT_1
            name: Project_1
            public: false
            scope: null
            type: NORMAL
        public: false
        relatedlinks: {}
        scmid: git
        scope: null
        slug: repo1
        state: AVAILABLE
        statusmessage: Available
```

Get repository with details in yaml format
```
$ bbctl repo get -k project_1 -s repo1 --show-details webhooks -o yaml
repositories:
    - projectKey: project_1
      repositorySlug: repo1
      webhooks:
        - id: 28
          active: true
          configuration: {}
          credentials:
            password: null
            username: myuser
          events:
            - repo:refs_changed
          name: build-hook
          scopetype: repository
          sslverificationrequired: true
          statistics: {}
          url: https://ci.example.com/webhook1

```

Create repositories from YAML
```
$ bbctl repo create -i examples/repos/create.yaml 
time=2025-08-21T15:16:36.381+03:00 level=INFO msg="Created repository" slug=repo1 name=repo1
time=2025-08-21T15:16:36.389+03:00 level=INFO msg="Created repository" slug=repo2 name=repo2

```

Create webhooks for repositories
```
$ bbctl repo webhook create -i examples/repos/webhooks/create.yaml
time=2025-08-21T15:24:26.911+03:00 level=INFO msg="Created webhook" project=project_1 repo=repo2 id=40 name=build-hook url=https://ci.example.com/webhook2
time=2025-08-21T15:24:26.922+03:00 level=INFO msg="Created webhook" project=project_1 repo=repo1 id=39 name=build-hook url=https://ci.example.com/webhook1

```


Create required builds for repositories
```
$ bbctl repo required-build create -i examples/repos/required-builds/create.yaml 
time=2025-08-21T15:37:30.690+03:00 level=INFO msg="Created required build merge check" project=project_1 slug=repo2 buildKey=14
time=2025-08-21T15:37:30.697+03:00 level=INFO msg="Created required build merge check" project=project_1 slug=repo2 buildKey=15

```


## 💰 Support the project

[![Donate on Boosty](https://img.shields.io/badge/Boosty-Donate-orange?logo=boosty&logoColor=white)](https://boosty.to/vinisman/donate)

[![Donate TON via NowPayments](https://img.shields.io/badge/Donate‑TON‑NowPayments-blue?logo=cryptocurrency&logoColor=white)](https://nowpayments.io/payment/?iid=5065138397)
