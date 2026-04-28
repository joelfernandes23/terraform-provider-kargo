# Terraform Provider for Kargo

> [!WARNING]
> This provider is still in active development and should be treated as prerelease software.
>
> Breaking changes are expected before a stable release. Resource schemas, import formats, behavior, and version compatibility may change between releases. Pin provider versions carefully, review changelogs before upgrading, and avoid using this provider for critical production workflows until a stable release is published.

The Kargo Terraform provider manages [Kargo](https://kargo.io/) continuous delivery resources through Terraform. It talks to the Kargo API using the Connect JSON protocol and is currently developed against Kargo `v1.9.5+`.

## Current Status

Implemented so far:

- `kargo_project` resource
- `kargo_project` data source
- `kargo_warehouse` resource with image, Git, and Helm chart subscriptions

Planned next:

- `kargo_warehouse` data source
- `kargo_stage` resource and data source
- additional observability, documentation, and release hardening

## Example Usage

```terraform
terraform {
  required_providers {
    kargo = {
      source  = "joelfernandes23/kargo"
      # Pin to a released version that includes the resources you use.
      # This provider is prerelease, so review changelogs before upgrading.
      # version = "0.x.y"
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
