# Changelog

All notable changes to this provider are documented in this file.

## 1.0.1 (2026-11-27)

First release of the maintained fork of
[mcfitz2/terraform-provider-pbs](https://github.com/mcfitz2/terraform-provider-pbs),
published as `laurinstreng/pbs`. Switch existing configurations with
`terraform state replace-provider registry.terraform.io/mcfitz2/pbs registry.terraform.io/laurinstreng/pbs`.

### Features

- `pbs_sync_job`: local datastore-to-datastore sync jobs (omit `remote`);
  switching between local and remote is an in-place update; `push` requires
  a `remote` and is validated at plan time.
- `digest` is now populated on datastores, remotes and jobs (informational;
  it is not used for optimistic locking).

### Bug fixes

- All resources: objects deleted outside Terraform are removed from state and
  re-created instead of failing every plan.
- Clearing optional attributes (comments, notification fields, S3 endpoint
  options, matcher targets, metrics comment) now removes them on the server.
- No more perpetual diffs for values PBS omits at their default (datastore
  `disabled`, notification `disable`/`mode`/`port`, metrics defaults).
- `pbs_metrics_server`: import works (`<type>/<name>`); the `server`/`port`
  style no longer causes diffs.
- Remote data sources (`pbs_remote_stores`, `pbs_remote_namespaces`,
  `pbs_remote_groups`) work again; store names are no longer empty and
  datastores in maintenance mode no longer break the scan.
- `pbs_webhook_notification`: headers and body are sent in the format PBS
  expects.
- `pbs_sync_job`: rate/burst values such as `10MiB` no longer cause
  inconsistent results.
- Documentation: correct API token format `user@realm!tokenname:secret`.

### Breaking changes

- `pbs_webhook_notification`: `secret` is now a sensitive map of named secrets
  instead of a single string. PBS stores webhook secrets as named values that
  are referenced in the `body` and `headers` templates as
  `{{ secrets.<name> }}`; the previous string form was rejected by PBS
  ("Expected array - got scalar value") and never worked. Migration:
  replace `secret = "x"` with `secret = { name = "x" }` and reference it as
  `{{ secrets.name }}`. Secret values are never returned by PBS, so they cannot
  be imported; changes to secret names made outside Terraform are detected as
  drift.
- `pbs_notification_endpoint` data source: the always-empty `secret` attribute
  was replaced by `secret_names`, the list of secret names configured on a
  webhook endpoint (values are never exposed).
