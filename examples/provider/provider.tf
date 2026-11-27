terraform {
  required_providers {
    pbs = {
      source  = "laurinstreng/pbs"
      version = "~> 0.1"
    }
  }
}

provider "pbs" {
  endpoint  = "https://pbs.example.com:8007"
  api_token = var.pbs_api_token # "terraform@pbs!automation:xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx"

  # Alternatively use username/password:
  # username = "admin@pbs"
  # password = var.pbs_password

  insecure = false # true to skip TLS verification (self-signed certificates)
  timeout  = 30    # API timeout in seconds
}

variable "pbs_api_token" {
  type      = string
  sensitive = true
}
