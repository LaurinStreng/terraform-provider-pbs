# Terraform Provider for Proxmox Backup Server

[![CI](https://github.com/LaurinStreng/terraform-provider-pbs/actions/workflows/ci.yml/badge.svg)](https://github.com/LaurinStreng/terraform-provider-pbs/actions/workflows/ci.yml)
[![Acceptance tests](https://github.com/LaurinStreng/terraform-provider-pbs/actions/workflows/acceptance.yml/badge.svg)](https://github.com/LaurinStreng/terraform-provider-pbs/actions/workflows/acceptance.yml)
[![License](https://img.shields.io/badge/License-MPL--2.0-blue.svg)](https://opensource.org/licenses/MPL-2.0)
[![Go Report Card](https://goreportcard.com/badge/github.com/LaurinStreng/terraform-provider-pbs)](https://goreportcard.com/report/github.com/LaurinStreng/terraform-provider-pbs)

A Terraform provider for managing [Proxmox Backup Server](https://www.proxmox.com/en/proxmox-backup-server) (PBS) resources.

> [!NOTE]
> **This is an actively maintained fork** of
> [mcfitz2/terraform-provider-pbs](https://github.com/mcfitz2/terraform-provider-pbs),
> originally created by [mcfitz2](https://github.com/mcfitz2). All credit for the
> original design and implementation goes to them — thank you for building it!
>
> This fork continues development with bug fixes, dependency updates, and a
> reworked acceptance test suite. It is published under its own registry
> namespace, `laurinstreng/pbs`.

## Features

- **Datastores**: directory, removable and S3-backed datastores, including PBS 4.x options (maintenance mode, notification routing, tuning)
- **S3 endpoints**: S3-compatible storage backends (AWS, Backblaze B2, Scaleway, MinIO, …)
- **Jobs**: prune, sync (remote pull/push and local datastore-to-datastore) and verify jobs
- **Remotes**: remote PBS servers for sync jobs
- **Metrics**: InfluxDB HTTP/UDP metrics servers
- **Notifications**: SMTP, Gotify, Sendmail and webhook targets plus notification matchers
- **Data sources** for all of the above
- Full lifecycle support: create, read, update, delete and import

### Planned

Not supported yet, but on the roadmap (contributions welcome — open an issue to coordinate):

- [ ] **Access control**: users, API tokens, ACLs / permissions
- [ ] **Authentication realms**: LDAP, Active Directory, OpenID Connect
- [ ] **Datastore namespaces**
- [ ] **Traffic control** rules (bandwidth limits)
- [ ] **Tape**: drives, changers, media pools, tape backup jobs
- [ ] **Node configuration**: DNS, time zone, HTTP proxy, network interfaces
- [ ] **Certificates & ACME** accounts and domains
- [ ] **Disks**: ZFS pools and directory storage creation

## Requirements

- [Terraform](https://developer.hashicorp.com/terraform/install) >= 1.0 (or OpenTofu)
- [Proxmox Backup Server](https://www.proxmox.com/en/proxmox-backup-server) 4.x
- [Go](https://go.dev/doc/install) >= 1.25 (only for building from source)

## Usage

```hcl
terraform {
  required_providers {
    pbs = {
      source  = "laurinstreng/pbs"
      version = "~> 0.1"
    }
  }
}

provider "pbs" {
  endpoint = "https://pbs.example.com:8007"
  username = "admin@pbs"
  password = var.pbs_password # or: api_token = var.pbs_api_token
  insecure = false            # true for self-signed certificates
}
```

All provider settings can also be set via environment variables:

```bash
export PBS_ENDPOINT="https://pbs.example.com:8007"
export PBS_USERNAME="admin@pbs"
export PBS_PASSWORD="your-password"
export PBS_API_TOKEN="user@realm!tokenname:secret"  # alternative to username/password
export PBS_INSECURE="false"
```

### Migrating from `mcfitz2/pbs`

Change the provider `source` to `laurinstreng/pbs`, then point existing state at the new provider:

```bash
terraform init -upgrade
terraform state replace-provider registry.terraform.io/mcfitz2/pbs registry.terraform.io/laurinstreng/pbs
```

## Examples

### Directory datastore

```hcl
resource "pbs_datastore" "backup" {
  name        = "backup-store"
  path        = "/mnt/datastore/backup"
  comment     = "Primary backup datastore"
  gc_schedule = "daily"
}
```

### Removable datastore

```hcl
resource "pbs_datastore" "removable" {
  name           = "removable-backups"
  path           = "/mnt/removable/removable-backups"
  removable      = true
  backing_device = "7d6fe83c-4c01-4a33-9ad0-9bc0216fb3e3" # filesystem UUID reported by PBS
}
```

### S3-backed datastore

```hcl
resource "pbs_s3_endpoint" "aws" {
  id         = "aws-s3"
  endpoint   = "s3.us-east-1.amazonaws.com" # hostname only, no https://
  region     = "us-east-1"
  access_key = var.aws_access_key
  secret_key = var.aws_secret_key
}

resource "pbs_datastore" "s3_backup" {
  name      = "s3-backup"
  path      = "/datastore/s3-backup-cache" # local cache directory
  s3_client = pbs_s3_endpoint.aws.id
  s3_bucket = "my-backup-bucket"
}
```

### PBS 4.x datastore options

```hcl
resource "pbs_datastore" "advanced" {
  name = "pbs-advanced"
  path = "/mnt/datastore/pbs-advanced"

  notification_mode = "notification-system"
  notify = {
    gc    = "always"
    prune = "error"
    sync  = "never"
  }

  maintenance_mode = {
    type    = "read-only"
    message = "Scheduled maintenance window"
  }

  verify_new = true

  tuning = {
    chunk_order           = "inode"
    gc_atime_cutoff       = 3600
    gc_atime_safety_check = true
    gc_cache_capacity     = 512
    sync_level            = "file"
  }
}
```

More examples live in [examples/](examples/). Full documentation for every resource and data source is on the [Terraform Registry](https://registry.terraform.io/providers/laurinstreng/pbs/latest/docs).

## Development

```bash
git clone https://github.com/LaurinStreng/terraform-provider-pbs.git
cd terraform-provider-pbs

make build   # build the provider
make test    # unit tests
make lint    # golangci-lint
make docs    # regenerate docs/
```

### Acceptance tests

Acceptance tests run against a real PBS and create/delete real objects — **never use a production server**.
A disposable PBS plus helper services (InfluxDB, Gotify, webhook receiver) can be started with Docker:

```bash
docker compose up -d                 # PBS at https://localhost:8007 (admin@pbs / pbspbs)
cp testacc.env.example testacc.env   # adjust if you use your own PBS
./testacc --list                     # show available tests
./testacc --tier light               # tiers: light, medium, heavy, all
./testacc TestAccResourceDatastore   # run tests by name
docker compose down -v               # clean up
```

The PBS container uses the community image [`ayufan/proxmox-backup-server`](https://github.com/ayufan/pve-backup-server-dockerfiles);
some hardware-related features (removable devices, ZFS) are not available in it.
See [CLAUDE.md](CLAUDE.md) for test conventions.

## Contributing

Contributions are welcome! Please:

1. Fork the repository and create a feature branch
2. Add or update acceptance tests for your change
3. Run `make lint`, `make test` and the relevant `./testacc` tests
4. Open a pull request

Bugs and feature requests: [GitHub Issues](https://github.com/LaurinStreng/terraform-provider-pbs/issues).

## Security

Secret scanning runs via a [gitleaks](https://github.com/gitleaks/gitleaks) pre-commit hook.
Please report vulnerabilities privately through [GitHub Security Advisories](https://github.com/LaurinStreng/terraform-provider-pbs/security/advisories/new).

## Acknowledgments

- [mcfitz2](https://github.com/mcfitz2) — original author of [terraform-provider-pbs](https://github.com/mcfitz2/terraform-provider-pbs)
- [bpg/terraform-provider-proxmox](https://github.com/bpg/terraform-provider-proxmox) — inspiration for the test setup
- The [Proxmox Backup Server](https://www.proxmox.com/en/proxmox-backup-server) team
- [Terraform Plugin Framework](https://github.com/hashicorp/terraform-plugin-framework)

## License

Mozilla Public License 2.0 — see [LICENSE](LICENSE). Original work © mcfitz2 and contributors.
