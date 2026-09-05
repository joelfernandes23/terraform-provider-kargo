# Terraform Provider for Kargo

[![codecov](https://codecov.io/gh/joelfernandes23/terraform-provider-kargo/branch/main/graph/badge.svg)](https://codecov.io/gh/joelfernandes23/terraform-provider-kargo)
[![Terraform Registry](https://img.shields.io/badge/Terraform-Registry-7B42BC?logo=terraform)](https://registry.terraform.io/providers/joelfernandes23/kargo/latest)
[![License: MPL 2.0](https://img.shields.io/badge/License-MPL%202.0-brightgreen.svg)](LICENSE)

Manage [Kargo](https://kargo.io/) Projects, ProjectConfigs, Warehouses, and Stages with Terraform.

## Quick Start

```terraform
terraform {
  required_providers {
    kargo = {
      source  = "joelfernandes23/kargo"
      version = "~> 0.6"
    }
  }
}

provider "kargo" {
  api_url      = "https://kargo.example.com"
  bearer_token = var.kargo_bearer_token
}

resource "kargo_project" "example" {
  name = "example-project"
}
```

Run `terraform init`, followed by `terraform plan`.

> [!WARNING]
> This provider is prerelease software. Pin its version and review the changelog before upgrading.

## Supported Resources

- `kargo_project` resource and data source
- `kargo_project_config` resource and data source
- `kargo_warehouse` resource and data source
- `kargo_stage` resource and data source

See the [Terraform Registry documentation](https://registry.terraform.io/providers/joelfernandes23/kargo/latest/docs) for schemas and examples.

## Example Usage

```terraform
terraform {
  required_providers {
    kargo = {
      source  = "joelfernandes23/kargo"
      version = "~> 0.6"
    }
  }

  required_version = ">= 1.5.0"
}

provider "kargo" {
  api_url      = "https://kargo.example.com"
  bearer_token = var.kargo_bearer_token
}

resource "kargo_project" "example" {
  name = "example-project"
}

resource "kargo_warehouse" "example" {
  project = kargo_project.example.name
  name    = "app"

  subscription {
    image {
      repo_url               = "ghcr.io/example/app"
      semver_constraint      = "^1.0.0"
      tag_selection_strategy = "SemVer"
      platform               = "linux/amd64"
    }
  }
}

resource "kargo_project_config" "example" {
  project = kargo_project.example.name

  promotion_policy {
    stage_selector {
      name = "dev"
    }
    auto_promotion_enabled = true
  }
}
```

## Authentication

The provider supports either a bearer token or the Kargo admin password:

```terraform
provider "kargo" {
  api_url      = "https://kargo.example.com"
  bearer_token = var.kargo_bearer_token
}
```

For local development, environment variables can also be used:

```shell
export KARGO_API_URL="https://localhost:31443"
export KARGO_ADMIN_PASSWORD="admin"
export KARGO_INSECURE_SKIP_TLS_VERIFY="true"
```

## Local Development

The development environment installs Kargo chart version `1.9.5`.

```shell
make devenv-up
make test
make lint
```

Run acceptance tests against the local environment:

```shell
export TF_ACC=1
go test -v -count=1 -parallel=4 -timeout 30m ./...
```

## Documentation

Generated Terraform Registry-style docs live in [`docs/`](docs/). Regenerate them with:

```shell
make docs
```

## Compatibility

The provider is currently developed and tested against Kargo `v1.9.5`. Compatibility with newer Kargo versions is intended, but not guaranteed while the provider is prerelease.
