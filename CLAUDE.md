# CLAUDE.md

Guidance for Claude Code (and humans) working in this repository.

## Project

Terraform provider for **Proxmox Backup Server (PBS)**, built on the
[Terraform Plugin Framework](https://developer.hashicorp.com/terraform/plugin/framework).

| Path | Purpose |
|------|---------|
| `main.go` | Provider entry point (`registry.terraform.io/laurinstreng/pbs`) |
| `fwprovider/provider.go` | Provider schema, configuration, resource/data source registration |
| `fwprovider/resources/<area>/` | Resources, one file per resource, tests next to them |
| `fwprovider/datasources/<area>/` | Data sources, tests next to them |
| `fwprovider/test/` | Shared acceptance test helpers (`InitEnvironment`, `CheckDestroy`, ...) |
| `pbs/` | PBS API client; `pbs/api` is the HTTP layer, other packages wrap endpoints |
| `docs/`, `examples/` | Registry documentation and examples (generated with `make docs`) |

## Commands

```bash
make build        # build the provider binary
make test         # unit tests (no PBS required)
make lint         # golangci-lint — run before committing; don't hand-format, use the linter
make docs         # regenerate docs/ via tfplugindocs
./testacc ...     # acceptance tests against a real PBS (see below)
```

## Acceptance tests (`./testacc`)

```bash
docker compose up -d                 # local PBS + InfluxDB + Gotify + webhook receiver
cp testacc.env.example testacc.env   # once; adjust when using your own PBS
./testacc --list                     # show all tests with tier/resource
./testacc TestAccResourceDatastore   # by name (regex, anchored with ^, `|` alternation)
./testacc --tier light               # light | medium | heavy | all, comma-separated
./testacc --resource datastore,jobs  # by resource annotation
./testacc TestAccX -- -count 2       # flags after -- go to `go test`
docker compose down -v               # discard all PBS state
```

`testacc.env` is git-ignored. Never point acceptance tests at a production PBS:
they create and delete real objects.

### Tiers

Every `TestAcc*` function carries two annotations directly above `func`:

```go
//testacc:tier=light
//testacc:resource=notifications
func TestAccResourceGotifyNotification(t *testing.T) {
```

| Tier | Scope | Parallelism |
|------|-------|-------------|
| `light` | Config-only objects, no datastores (users/tokens/ACLs, notifications, metrics, remotes, endpoints, validation) | `-p 8` |
| `medium` | Creates datastores or objects that depend on them (datastores, jobs) | `-p 4` |
| `heavy` | Needs external services/credentials (S3); skipped when credentials are unset | `-p 1` |

Resource annotations: `access`, `datastore`, `endpoints`, `jobs`, `metrics`, `notifications`, `remotes`.

### Writing tests

- **Location:** `<name>_test.go` next to the resource/data source, in the external
  test package (`package datastores_test`). Use `fwprovider/test` helpers;
  see `fwprovider/resources/datastores/datastore_test.go` as the reference.
- **Names:** `TestAccResource<Name>…` / `TestAccDataSource<Name>…`, describing behavior.
  No issue numbers in test or object names.
- **Object names:** always `te.RandomName("<kind>")` (prefix `tfacc-`), never fixed names,
  so tests can run in parallel and leftovers are identifiable.
- **Configs:** `te.RenderConfig` with `{{.Var}}` template variables; helper-service
  addresses (`{{.InfluxDBHost}}`, `{{.GotifyHost}}`, …) come from `testacc.env`.
- **Coverage per resource:** create → verify, update in place (assert with
  `plancheck.ExpectResourceAction(..., ResourceActionUpdate)`), import round-trip
  (`ImportStateVerify`), replacement for immutable fields (`ResourceActionReplace`),
  `CheckDestroy` via the API client, and out-of-band deletion (resource must be
  removed from state and re-created, not error).
- **Verify against the server, not only state:** use `te.Client()` in `Check`
  functions to read the object from PBS and compare (`te.APIClient()` for raw
  endpoints that `pbs.Client` doesn't wrap).
- **Remotes/sync jobs:** `te.AddSelfRemoteVars()` provides a `pbs_remote` pointing
  at the PBS under test (`{{.RemoteHost}}`, `{{.RemoteAuthID}}`, `{{.RemotePassword}}`,
  `{{.RemoteFingerprint}}`), so no second server is needed.
- **Tests must fail without the fix.** When fixing a bug, first write/run the test
  and see it fail.
- **No fakes as proof of behavior:** no `httptest` servers, stub clients, or
  hand-built API responses in acceptance tests. Unit tests for pure functions
  (parsers, validators, `pbs/api` helpers) are fine.
- **`ImportStateVerifyIgnore`** only for attributes PBS genuinely cannot return
  (write-only secrets, create-only flags); add a comment explaining why.
- If the test PBS is unreachable, ask the user instead of replacing acceptance
  tests with unit tests.

## Code conventions

- Read must call `resp.State.RemoveResource(ctx)` when `api.IsNotFound(err)`.
- PBS omits false/empty values in responses; map absence to the schema default
  in Read, otherwise users get perpetual diffs.
- Immutable attributes use `RequiresReplace()` plan modifiers.
- Clearing an optional attribute must send it in PBS's `delete` list on update;
  omitting it leaves the old value on the server.
- `digest` is the SHA-256 of the whole config file (e.g. `sync.cfg`), shared by all
  objects in it. Expose it as Computed (no `UseStateForUnknown`), never send it back
  for optimistic locking — that causes false conflicts when Terraform updates several
  objects of one type in a single apply. Re-read the object after Create/Update.
- Errors: `resp.Diagnostics.AddError("Error <Verb>ing <Thing>", "...: "+err.Error())`.
- Every Go file starts with the MPL-2.0 header.

## Release

Tag `vX.Y.Z` on `main` → `.github/workflows/release.yml` runs GoReleaser and
signs with the `GPG_PRIVATE_KEY`/`GPG_PASSPHRASE` secrets. Registry
users pin `~> 1.0`; follow semantic versioning and record changes in
`CHANGELOG.md` before tagging.
