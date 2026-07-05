provider "kargo" {}

resource "kargo_project" "example" {
  name = "example-project"
}

resource "kargo_warehouse" "example" {
  project = kargo_project.example.name
  name    = "app"

  subscription {
    image {
      repo_url          = "ghcr.io/example/app"
      semver_constraint = "^1.0.0"
    }
  }
}

resource "kargo_stage" "test" {
  project = kargo_project.example.name
  name    = "test"

  requested_freight {
    origin {
      kind = "Warehouse"
      name = kargo_warehouse.example.name
    }
    sources {
      direct = true
    }
  }

  promotion_template {
    step {
      uses = "git-clone"
      as   = "clone"
      config = jsonencode({
        checkout = [{ branch = "main", path = "./src" }]
        repoURL  = "https://github.com/example/app-config.git"
      })
    }
  }
}

resource "kargo_stage" "prod" {
  project = kargo_project.example.name
  name    = "prod"
  shard   = "eu-west"

  requested_freight {
    origin {
      kind = "Warehouse"
      name = kargo_warehouse.example.name
    }
    sources {
      stages = [kargo_stage.test.name]
    }
  }

  promotion_template {
    step {
      uses = "argocd-update"
      config = jsonencode({
        apps = [{ name = "app-prod" }]
      })
    }
  }
}
