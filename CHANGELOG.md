# Changelog

All notable changes to this provider are documented in this file.

## Unreleased

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
