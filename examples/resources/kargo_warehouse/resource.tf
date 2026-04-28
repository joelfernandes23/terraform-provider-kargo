provider "kargo" {}

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

  subscription {
    git {
      repo_url          = "https://github.com/example/app-config.git"
      branch            = "main"
      semver_constraint = "^1.0.0"
    }
  }

  subscription {
    chart {
      repo_url          = "https://charts.example.com"
      name              = "app"
      semver_constraint = "^1.0.0"
    }
  }
}
