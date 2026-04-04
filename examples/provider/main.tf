terraform {
  required_providers {
    kargo = {
      source = "joelfernandes23/kargo"
    }
  }

  required_version = ">= 1.5.0"
}

provider "kargo" {
  api_url                  = "https://localhost:31443"
  insecure_skip_tls_verify = true
}
