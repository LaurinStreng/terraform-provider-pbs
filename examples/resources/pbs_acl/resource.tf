resource "pbs_user" "backup" {
  userid = "backup@pbs"
}

resource "pbs_api_token" "backup" {
  userid     = pbs_user.backup.userid
  token_name = "client"
}

# Allow the user to back up into the datastore "store1".
resource "pbs_acl" "backup_user" {
  path    = "/datastore/store1"
  auth_id = pbs_user.backup.userid
  role    = "DatastoreBackup"
}

# Grant the API token read access to a single namespace only.
resource "pbs_acl" "backup_token" {
  path      = "/datastore/store1/clients"
  auth_id   = pbs_api_token.backup.tokenid
  role      = "DatastoreReader"
  propagate = false
}
