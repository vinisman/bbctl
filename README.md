# bbctl

**bbctl** is a CLI tool for managing repositories and automation in **Bitbucket Server / Data Center** environments.  
It provides streamlined support for repository creation, deletion, and updating 

## ✨ Features

- Management multiple repositories from multiple projects
- Getting additional information about repository using manifest file from the root of the repository  
- Parallel processing for high-performance bulk operations
- YAML input/output for full GitOps compatibility
- Easy configuration via `.env` file

## Configuration
Add .env properties file like below or provide command line keys. For details see --help
```
BITBUCKET_BASE_URL=https://bitbucket-server.local
BITBUCKET_TOKEN=<token>
```


## Examples
1) List repos
```
$ bbctl get repos -p PRJ
Name        Archived  State      Project
my repo     false     AVAILABLE  PRJ
myrepo      false     AVAILABLE  PRJ
new repo    false     AVAILABLE  PRJ
new repo 1  false     AVAILABLE  PRJ
test        false     AVAILABLE  PRJ
test-1      false     AVAILABLE  PRJ

$ bbctl get repo test -p PRJ
Name  Archived  State      Project
test  false     AVAILABLE  PRJ

```
2) Create repos
```
$ cat out/create_repos.yaml 
repos:
  - name: "Repo1"
    project: "PRJ"
    description: "My first repo 1111"
  - name: "Repo2"
    slug: "my-repo-2"
    project: "PRJ"

$ bbctl create repos -f out/create_repos.yaml 
time=2025-08-11T15:15:09.064+03:00 level=INFO msg="Repository created successfully" name=Repo1 slug=""
time=2025-08-11T15:15:09.071+03:00 level=INFO msg="Repository created successfully" name=Repo2 slug=my-repo-2

```
3) Delete repos
```
$ cat out/delete_repos.yaml 
repos:
  - slug: "repo1"
    project: "PRJ"
  - slug: "my-repo-2"

$ bbctl delete repos -f out/repos.yaml 
time=2025-08-11T15:16:46.120+03:00 level=ERROR msg="Repository slug is missing" project=PRJ
time=2025-08-11T15:16:46.279+03:00 level=INFO msg="Repository deleted successfully" project=PRJ slug=my-repo-2

```

4) Update repos
```
$ cat out/update_repos.yaml 
repos:
  - name: "Repo1"
    slug: "repo1"
    project: "PRJ"
    description: "My first repo"
  - name: "Repo2"
    slug: "repo2"
    project: "PRJ"

$ bbctl apply repos -f out/update_repos.yaml 
time=2025-08-11T15:24:33.724+03:00 level=INFO msg="Repository updated successfully" name=Repo2 slug=repo2
time=2025-08-11T15:24:33.799+03:00 level=INFO msg="Repository updated successfully" name=Repo1 slug=repo1

```


## 💰 Support the project

[![Donate on Boosty](https://img.shields.io/badge/Boosty-Donate-orange?logo=boosty&logoColor=white)](https://boosty.to/vinisman/donate)

[![Donate TON via NowPayments](https://img.shields.io/badge/Donate‑TON‑NowPayments-blue?logo=cryptocurrency&logoColor=white)](https://nowpayments.io/payment/?iid=5065138397)
