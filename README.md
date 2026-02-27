# bbctl

**bbctl** is a CLI tool for managing repositories, projects, and users in **Bitbucket Server / Data Center** environments.  
It provides streamlined support for creating, deleting, updating, and retrieving information about projects, repositories, and users.

## ✨ Features

- Manage multiple repositories, projects, and users
- Retrieve additional repository information using a manifest file from the root of the repository
- Parallel processing for high-performance bulk operations
- YAML/JSON output for full GitOps compatibility
- Easy configuration via `.env` file
- Support reading YAML/JSON from stdin (`-`) for all relevant commands
- Unified project file format across all project operations
- User management with secure password handling
- **File validation** against JSON Schema (YAML/JSON support)

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
- **Create** new projects (with optional YAML/JSON output)
- **Delete** existing projects
- **Update** project information (with optional YAML/JSON output)
- **Retrieve basic info** about projects (plain/YAML/JSON formats)

### For repositories
- **Create** new repositories
- **Delete** existing repositories
- **Create forks** of repositories
- **Update** repository information (including moving repositories between projects)
- **Retrieve basic info** about repositories
- **Retrieve detailed info** for repositories, including:
  - Webhooks
  - Required builds
  - Reviewer groups
  - Manifest file information (from the root of the repository)
  - Default branch
- **Workzone plugin management** for repositories:
  - **Properties**: Repository workflow properties
  - **Reviewers**: Branch reviewers list
  - **Signatures**: Branch sign approvers list
  - **Mergerules**: Branch automergers list

### For users
- **Create** new users (with secure password handling)
- **Delete** existing users
- **Update** user information (display name, email address)
- **Retrieve basic info** about users (plain/YAML/JSON formats)
- **Bulk operations** for managing multiple users

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
```

Create project with output
```
$ bbctl project create -k demo_project --name demoProject -o yaml
projects:
  - id: 54
    key: DEMO_PROJECT
    name: demoProject
    description: ""
    public: false
    type: NORMAL
```

Bulk create projects from yaml file
```
$ bbctl project create -i examples/projects/create.yaml
```

Bulk create projects with output
```
$ bbctl project create -i examples/projects/create.yaml -o json
{
  "projects": [
    {
      "id": 55,
      "key": "D_PROJECT_1",
      "name": "Demo project 1",
      "description": "Description for project 1",
      "public": false,
      "type": "NORMAL"
    }
  ]
}
```

Update project from CLI
```
$ bbctl project update -k demo_project --name "Updated Project" --description "New description"
```

Update projects from yaml file
```
$ bbctl project update -i projects.yaml
```

Update projects with output
```
$ bbctl project update -i projects.yaml -o yaml
projects:
  - id: 54
    key: DEMO_PROJECT
    name: Updated Project
    description: New description
    public: false
    type: NORMAL
```

Delete project from CLI
```
$ bbctl project delete -k demo_project
```

Delete projects from yaml file
```
$ bbctl project delete -i projects.yaml
```

Example project yaml file format
```yaml
projects:
  - key: PROJECT1
    name: Project 1
    description: Description for project 1
  - key: PROJECT2
    name: Project 2
    description: Description for project 2
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
$ bbctl repo get -s PROJECT_1/repo1 -o yaml
repositories:
    - projectKey: PROJECT_1
      repositorySlug: repo1
      restRepository:
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
$ bbctl repo get -s PROJECT_1/repo1 --show-details webhooks -o yaml
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

Get only repository config files as separate sections
```
$ bbctl repo get -s PROJECT_1/repo1 --show-details configs --config-file manifest=manifest.json --config-file config=config.yaml -o yaml
repositories:
  - projectKey: PROJECT_1
    repositorySlug: repo1
    manifest:
      version: 1
      services:
        - api
    config:
      build:
        template: kotlin
```

Show only defaultBranch (flat field) without full repository payload
```
$ bbctl repo get -s PROJECT_1/repo1 --show-details defaultBranch -o json
{
  "repositories": [
    {
      "projectKey": "PROJECT_1",
      "repositorySlug": "repo1",
      "defaultBranch": "refs/heads/main"
    }
  ]
}
```

Notes about --show-details:
- If you pass `--show-details`, only the listed sections are included in the output (repository, defaultBranch, webhooks, required-builds, manifest, configs).
- `defaultBranch` is output as a top-level field `defaultBranch`. If `repository` is also requested, it is additionally written into `restRepository.defaultBranch`.
- An explicitly empty value is invalid: `--show-details ""` will return an error.
- `--manifest-file` keeps legacy behavior and fills the `manifest` section.
- `--config-file` can be repeated (or passed comma-separated). Format: `key=filepath`. Each file is output as a separate top-level section using the specified key, e.g. `--config-file manifest=manifest.json`.

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

## Reviewer Groups Management Examples

Reviewer groups allow you to configure default reviewers for pull requests at the repository level. bbctl supports creating, updating, deleting, and retrieving reviewer groups.

### Get Reviewer Groups

Get all reviewer groups for a repository
```bash
$ bbctl repo reviewer-group get -s PROJECT_1/repo1 -o yaml
repositories:
  - projectKey: PROJECT_1
    repositorySlug: repo1
    reviewerGroups:
      - id: 1
        name: senior-developers
        description: "Senior developers for code review"
        users:
          - name: john.doe
            id: 123
          - name: jane.smith
            id: 124
```

Get reviewer groups for multiple repositories
```bash
$ bbctl repo reviewer-group get -s PROJECT_1/repo1,PROJECT_1/repo2 -o json
```

Get reviewer groups from YAML file
```bash
$ bbctl repo reviewer-group get -i repositories.yaml -o yaml
```

### Create Reviewer Groups

Create reviewer groups from YAML file
```bash
$ bbctl repo reviewer-group create -i examples/repos/reviewer-groups/create.yaml
time=2025-11-06T18:05:09.258+03:00 level=INFO msg="Created reviewer group" project=PROJECT_1 repo=repo1 name=senior-developers id=1
time=2025-11-06T18:05:09.297+03:00 level=INFO msg="Created reviewer group" project=PROJECT_1 repo=repo1 name=team-leads id=2
```

Example YAML file for creating reviewer groups (`examples/repos/reviewer-groups/create.yaml`):
```yaml
repositories:
  - projectKey: DEV
    repositorySlug: my-repo
    reviewerGroups:
      - name: senior-developers
        description: "Senior developers for code review"
        users:
          # Option 1: Only username (ID will be fetched automatically)
          - name: john.doe
          - name: jane.smith
      - name: team-leads
        description: "Team leads group"
        users:
          # Option 2: Username with ID (no additional API call needed)
          - name: alice.johnson
            id: 123
          - name: bob.wilson
            id: 124
```

**Notes:**
- Users can be specified with just `name` (bbctl will automatically fetch the user ID)
- Alternatively, you can provide both `name` and `id` to avoid additional API calls
- Only `name` and `id` fields are required by Bitbucket API

### Update Reviewer Groups

Update reviewer groups from YAML file (requires ID for each group)
```bash
$ bbctl repo reviewer-group update -i examples/repos/reviewer-groups/update.yaml
time=2025-11-06T18:09:17.405+03:00 level=INFO msg="Updated reviewer group" project=DEV repo=my-repo name=senior-developers id=1
```

Example YAML file for updating reviewer groups (`examples/repos/reviewer-groups/update.yaml`):
```yaml
repositories:
  - projectKey: DEV
    repositorySlug: my-repo
    reviewerGroups:
      - id: 1
        name: senior-developers
        description: "Updated: Senior developers for code review"
        users:
          - name: john.doe
          - name: jane.smith
          - name: new.developer
```

**Note:** Each reviewer group must have an `id` field to identify which group to update.

### Delete Reviewer Groups

Delete reviewer groups by ID from command line
```bash
$ bbctl repo reviewer-group delete -k PROJECT_1 -s repo1 --ids 1,2
time=2025-11-06T18:09:34.545+03:00 level=INFO msg="Deleted reviewer group" project=PROJECT_1 repo=repo1 id=1
time=2025-11-06T18:09:34.546+03:00 level=INFO msg="Deleted reviewer group" project=PROJECT_1 repo=repo1 id=2
```

Delete reviewer groups from YAML file
```bash
$ bbctl repo reviewer-group delete -i examples/repos/reviewer-groups/delete.yaml
```

Example YAML file for deleting reviewer groups (`examples/repos/reviewer-groups/delete.yaml`):
```yaml
repositories:
  - projectKey: DEV
    repositorySlug: my-repo
    reviewerGroups:
      - id: 1
      - id: 2
```

### Notes about Reviewer Groups

- **User ID enrichment**: If users are specified with only `name`, bbctl automatically fetches their IDs from Bitbucket
- **Performance**: Providing both `name` and `id` in YAML avoids additional API calls
- **Required fields**: Bitbucket API requires only `name` and `id` fields for users in reviewer groups
- **Parallel processing**: All operations support parallel processing when working with multiple repositories
- **Output formats**: All commands support `plain`, `yaml`, and `json` output formats

### GitOps: diff/apply for required-builds
Compare two YAML/JSON files (source = current state, target = desired state), get a structured diff (create/update/delete), optionally apply to Bitbucket, save rollback plan, and export results.

```
# Show diff
bbctl repo required-build diff \
  --source out/task1/rb_v1.json \
  --target out/task1/rb_v2.json -o json

# Apply diff (delete -> update -> create) and save rollback plan
bbctl repo required-build diff \
  --source out/task1/rb_v1.json \
  --target out/task1/rb_v2.json \
  --apply -o yaml \
  --apply-rollback-out out/rollback-required-builds.yaml

# Save only created+updated items (grouped by repo) to file, stdout unchanged
bbctl repo required-build diff \
  --source s.json --target t.json \
  --apply -o json \
  --apply-result-out out/created-updated-required-builds.json

# Force all target items whose id exists in source into update section
bbctl repo required-build diff --source s.json --target t.json --force-update -o json

# Rollback previously applied changes
bbctl repo required-build diff --rollback out/rollback-required-builds.yaml -o json
```

Notes:
- --apply-result-out accepts only a file path; output format is controlled by -o (json/yaml). Stdout prints the normal apply result.
- --apply-rollback-out and --apply-result-out are valid only together with --apply.
- Update operations skip 404 (missing item) gracefully.

### GitOps: diff/apply for webhooks
The same workflow is available for webhooks.

```
# Show diff
bbctl repo webhook diff --source hooks_v1.json --target hooks_v2.json -o json

# Apply and save rollback plan
bbctl repo webhook diff \
  --source hooks_v1.json --target hooks_v2.json \
  --apply -o yaml \
  --apply-rollback-out out/rollback-webhooks.yaml

# Save only created+updated webhooks to file (grouped by repo)
bbctl repo webhook diff \
  --source hooks_v1.json --target hooks_v2.json \
  --apply -o json \
  --apply-result-out out/created-updated-webhooks.json

# Force-update by id
bbctl repo webhook diff --source hooks_v1.json --target hooks_v2.json --force-update -o json

# Rollback
bbctl repo webhook diff --rollback out/rollback-webhooks.yaml -o json
```

## User Management Examples

List users in plain format
```
$ bbctl user get -n user1,user2
Name    Display Name    Email Address    Active
user1   User One        user1@example.com    true
user2   User Two        user2@example.com    true
```

List all users in YAML format
```
$ bbctl user get --all -o yaml
users:
  - name: user1
    displayName: User One
    emailAddress: user1@example.com
    active: true
  - name: user2
    displayName: User Two
    emailAddress: user2@example.com
    active: true
```

Create user from CLI
```
$ bbctl user create -n user1 --displayName "User One" --email user1@example.com --user-password SecurePass123!
```

Create users from YAML file
```
$ bbctl user create -i examples/users/create.yaml --user-password SecurePass123!
time=2025-09-15T22:30:15.123+03:00 level=INFO msg="Created user" username=user1
time=2025-09-15T22:30:15.456+03:00 level=INFO msg="Created user" username=user2
```

Update user information
```
$ bbctl user update -n user1 --displayName "Updated User One" --email user1@newdomain.com
time=2025-09-15T22:35:20.789+03:00 level=INFO msg="Updated user" username=user1
```

Update users from YAML file
```
$ bbctl user update -i examples/users/update.yaml
time=2025-09-15T22:40:10.123+03:00 level=INFO msg="Updated user" username=user1
time=2025-09-15T22:40:10.456+03:00 level=INFO msg="Updated user" username=user2
```

Delete user
```
$ bbctl user delete -n user1
time=2025-09-15T22:45:30.123+03:00 level=INFO msg="Deleted user" username=user1
```

Delete users from YAML file
```
$ bbctl user delete -i examples/users/delete.yaml
time=2025-09-15T22:50:15.123+03:00 level=INFO msg="Deleted user" username=user1
time=2025-09-15T22:50:15.456+03:00 level=INFO msg="Deleted user" username=user2
```

Example user YAML file format
```yaml
users:
  - name: user1
    displayName: "User One"
    emailAddress: user1@example.com
  - name: user2
    displayName: "User Two"
    emailAddress: user2@example.com
```

## Workzone Plugin Management Examples

The Workzone plugin provides advanced repository workflow management capabilities. bbctl supports all four main sections of the Workzone plugin.

### Get Workzone settings

Get all Workzone settings for a repository
```bash
$ bbctl repo workzone get -s PROJECT_1/repo1 -o yaml
repositories:
  - projectKey: PROJECT_1
    repositorySlug: repo1
    workzone:
      workflowProperties:
        enableWorkflow: true
        workflowName: "Standard Workflow"
      reviewers:
        - refName: refs/heads/main
          users: [{ name: "reviewer1" }]
          groups: ["developers"]
      signapprovers:
        - refName: refs/heads/main
          users: [{ name: "signer1" }]
          groups: ["seniors"]
      mergerules:
        - refName: refs/heads/main
          approvalQuotaEnabled: true
          mergeStrategyId: "squash"
```

Get specific sections only
```bash
$ bbctl repo workzone get -s PROJECT_1/repo1 --section properties,reviewers -o yaml
```

Get settings for multiple repositories
```bash
$ bbctl repo workzone get -s PROJECT_1/repo1,PROJECT_1/repo2 --section mergerules -o yaml
```

### Set Workzone settings

Set workflow properties for a repository
```bash
$ bbctl repo workzone set -s PROJECT_1/repo1 --section properties -i examples/repos/workzone/properties.yaml
INFO Successfully set workflow properties for 1 repositories
INFO All 1 sections completed successfully for 1 repositories
```

Set multiple sections from YAML file
```bash
$ bbctl repo workzone set --section properties,reviewers,signatures,mergerules -i workzone-settings.yaml
INFO Successfully set workflow properties for 5 repositories
INFO Successfully set reviewers for 5 repositories
INFO Successfully set sign approvers for 5 repositories
INFO Successfully set mergerules for 5 repositories
INFO All 4 sections completed successfully for 5 repositories
```

### Update Workzone settings

Update workflow properties (merge with existing)
```bash
$ bbctl repo workzone update -s PROJECT_1/repo1 --section properties -i updated-properties.yaml
INFO Successfully updated workflow properties for 1 repositories
INFO All 1 sections updated successfully for 1 repositories
```

Replace lists (reviewers, signatures, mergerules)
```bash
$ bbctl repo workzone update --section reviewers,signatures -i new-lists.yaml
INFO Successfully updated reviewers for 3 repositories
INFO Successfully updated sign approvers for 3 repositories
INFO All 2 sections updated successfully for 3 repositories
```

### Delete Workzone settings

Delete specific sections
```bash
$ bbctl repo workzone delete -s PROJECT_1/repo1 --section reviewers,signatures
INFO Successfully deleted reviewers for 1 repositories
INFO Successfully deleted sign approvers for 1 repositories
INFO All 2 sections deleted successfully for 1 repositories
```

Delete from multiple repositories
```bash
$ bbctl repo workzone delete --section mergerules -i repos-to-clean.yaml
INFO Successfully deleted mergerules for 10 repositories
INFO All 1 sections deleted successfully for 10 repositories
```

### Example YAML files

**Properties** (`examples/repos/workzone/properties.yaml`):
```yaml
repositories:
  - projectKey: PROJECT
    repositorySlug: repo
    workzone:
      workflowProperties:
        enableWorkflow: true
        workflowName: "Standard Workflow"
        autoMergeEnabled: false
```

**Reviewers** (`examples/repos/workzone/reviewers.yaml`):
```yaml
repositories:
  - projectKey: PROJECT
    repositorySlug: repo
    workzone:
      reviewers:
        - refName: refs/heads/main
          users:
            - { name: "reviewer1" }
            - { name: "reviewer2" }
          groups:
            - "developers"
            - "seniors"
        - refName: refs/heads/develop
          users:
            - { name: "lead" }
          groups:
            - "tech-leads"
```

**Signatures** (`examples/repos/workzone/signatures.yaml`):
```yaml
repositories:
  - projectKey: PROJECT
    repositorySlug: repo
    workzone:
      signapprovers:
        - refName: refs/heads/main
          users:
            - { name: "signer1" }
            - { name: "signer2" }
          groups:
            - "seniors"
        - refName: refs/heads/release/*
          users:
            - { name: "release-manager" }
          groups:
            - "release-team"
```

**Mergerules** (`examples/repos/workzone/mergerules.yaml`):
```yaml
repositories:
  - projectKey: PROJECT
    repositorySlug: repo
    workzone:
      mergerules:
        - projectkey: PROJECT
          reposlug: repo
          refname: refs/heads/trunk
          refpattern: null
          srcrefname: null
          srcrefpattern: null
          automergeusers: []
          approvalquotaenabled: true
          approvalquota: "100"
          approvalcount: 0
          mandatoryapprovalcount: 0
          deletesourcebranch: false
          watchbuildresult: false
          watchtaskcompletion: false
          requiredbuildscount: 0
          requiredsignaturescount: 0
          groupquota: 0
          mergecondition: null
          mergestrategyid: none-inherit
          ignorecontributingreviewersapproval: false
          enableneedsworkveto: null
```

### Notes about Workzone commands

- **Parallel processing**: All commands support parallel processing when working with multiple repositories
- **Section selection**: Use `--section` to specify which Workzone sections to operate on (properties, reviewers, signatures, mergerules)
- **Default behavior**: If no `--section` is specified for `get` command, all sections are fetched
- **Input format**: All commands accept YAML/JSON input files or repository identifiers via `--repositorySlug`
- **Feedback**: Commands provide detailed feedback about successful operations and error handling
- **Plugin compatibility**: Section names match the Workzone plugin tab names for easy identification
- **Performance**: Fetching all sections by default may be slower than selecting specific sections, especially for repositories with many branches or complex configurations

## File Validation

Validate any JSON or YAML file against a JSON Schema.

### Basic validation

Validate a YAML file against a schema:
```bash
$ bbctl validate --schema schema.json --data data.yaml
✓ Validation passed
```

Validate a JSON file:
```bash
$ bbctl validate --schema schema.json --data data.json
✓ Validation passed
```

### Verbose error output

```bash
$ bbctl validate --schema schema.json --data invalid.yaml --verbose
Error: validation failed:
Validation errors:
validating root: validating /properties/email: pattern: "not-an-email" does not match pattern "^[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}$"
```

### JSON output

```bash
$ bbctl validate --schema schema.json --data data.yaml -o json
{
  "valid": true,
  "message": "Validation passed"
}
```

### Read from stdin

Read data from stdin:
```bash
$ cat data.yaml | bbctl validate --schema schema.json --data -
```

Read schema from stdin:
```bash
$ cat schema.json | bbctl validate --schema - --data data.yaml
```

### Example schema

```json
{
  "$schema": "http://json-schema.org/draft-07/schema#",
  "type": "object",
  "required": ["name", "email"],
  "properties": {
    "name": {
      "type": "string",
      "minLength": 1
    },
    "email": {
      "type": "string",
      "format": "email"
    },
    "age": {
      "type": "integer",
      "minimum": 0
    }
  }
}
```

Example data file (YAML):
```yaml
name: John Doe
email: john@example.com
age: 30
```


## 💰 Support the project

[![Donate on Boosty](https://img.shields.io/badge/Boosty-Donate-orange?logo=boosty&logoColor=white)](https://boosty.to/vinisman/donate)

[![Donate TON via NowPayments](https://img.shields.io/badge/Donate‑TON‑NowPayments-blue?logo=cryptocurrency&logoColor=white)](https://nowpayments.io/payment/?iid=5065138397)
