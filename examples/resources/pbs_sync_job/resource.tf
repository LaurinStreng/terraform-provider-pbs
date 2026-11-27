# Pull backups from a remote PBS into a local datastore
resource "pbs_sync_job" "offsite" {
  id               = "offsite-pull"
  store            = "backups"
  remote           = pbs_remote.offsite_pbs.name
  remote_store     = "datastore1"
  remote_namespace = "prod"
  schedule         = "hourly"
  remove_vanished  = true
  rate_in          = "50M"
  comment          = "Pull production backups from offsite PBS"
}

# Local sync job: copy one datastore into another datastore on the same PBS
# (PBS >= 3.3). Omit `remote`; `remote_store` is the local source datastore.
resource "pbs_sync_job" "local_to_ext" {
  id              = "local-to-ext"
  store           = "ext"   # target datastore
  remote_store    = "local" # source datastore on this PBS
  schedule        = "daily"
  remove_vanished = true
  comment         = "local -> ext, daily"
}

# Push sync job: push local backups to a remote PBS (requires `remote`)
resource "pbs_sync_job" "push" {
  id             = "push-offsite"
  store          = "backups"
  remote         = pbs_remote.offsite_pbs.name
  remote_store   = "datastore1"
  sync_direction = "push"
  schedule       = "daily"
}
