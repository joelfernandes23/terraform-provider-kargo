terraform {
  required_providers {
    kargo = {
      source  = "joelfernandes23/kargo"
      version = "~> 0.3"
    }
  }

  required_version = ">= 1.5.0"
}

variable "kargo_bearer_token" {
  type      = string
  sensitive = true
}

provider "kargo" {
  api_url      = "https://kargo.example.com"
  bearer_token = var.kargo_bearer_token
}
