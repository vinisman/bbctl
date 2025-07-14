# bbctl

**bbctl** is a CLI tool for managing repositories and automation in **Bitbucket Server / Data Center** environments.  
It provides streamlined support for repository creation, manifest management

## ✨ Features

- Management multiple repositories from YAML config
- All commands are run in the project context
- Parallel processing for high-performance bulk operations
- YAML input/output for full GitOps compatibility
- Easy configuration via `.env` file

## Configuration
Add .env properties file like below or provide command line keys. For details see --help
```
BITBUCKET_URL=https://bitbucket-server.local
BITBUCKET_TOKEN=<token>
```


## Examples
1) List repos
```
$ bbctl get repos -p PROJECT1
name
my-repo-4
my-repo-1
my-repo-2
my-repo-3

$ get repo my-repo-1 -p PROJECT1 --manifest-file manifest.json --template '{{.type}}'
name            type
my-repo-1       application

```
2) Create repos
```
$ cat repos_to_create.yaml
repositories:
  - name: my-repo-1
    defaultBranch: trunk
    description: My test repo
  - name: my-repo-2
    description: My test repo
  - name: my-repo-3
    defaultBranch: master
    description: Description

$ bbctl create repos -p PROJECT1 -i repos_to_create.yaml
```
3) Delete repos
```
$ cat repos_to_delete.yaml
repositories:
  - name: my-repo-1
  - name: my-repo-2
  - name: my-repo-3

$ bbctl delete repos -p PROJECT1 -i repos_to_delete.yaml
```

3) Update repos
```
$ cat repos_to_update.yaml
repositories:
  - name: my-repo-1
    defaultBranch: trunk
    description: My test repo new
  - name: my-repo-2
    description: My test repo new
  - name: my-repo-3
    defaultBranch: master
    description: Description

$ bbctl apply repos -p PROJECT1 -i repos_to_update.yaml
```


## 💰 Support the project

[![Donate on Boosty](https://img.shields.io/badge/Boosty-Donate-orange?logo=boosty&logoColor=white)](https://boosty.to/vinisman/donate)

[![Donate TON via NowPayments](https://img.shields.io/badge/Donate‑TON‑NowPayments-blue?logo=cryptocurrency&logoColor=white)](https://nowpayments.io/payment/?iid=5065138397)
