# bbctl

**bbctl** is a CLI tool for managing repositories and automation in **Bitbucket Server / Data Center** environments.  
It provides streamlined support for repository creation, manifest management, and CI/CD integration.

## ✨ Features

- Create multiple repositories from YAML config
- Automatically add `manifest.json` to repositories
- Sync required builds via Bitbucket REST API
- Parallel processing for high-performance bulk operations
- YAML input/output for full GitOps compatibility
- Easy configuration via `.env` file
- Written in Go — fast and extendable

## 📦 Installation

Clone the repo and build:

```bash
git clone https://github.com/vinisman/bbctl.git
cd bbctl
go build -o bbctl