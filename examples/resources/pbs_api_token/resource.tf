resource "pbs_user" "backup" {
  userid  = "backup@pbs"
  comment = "Automation user"
}

resource "pbs_api_token" "backup" {
  userid     = pbs_user.backup.userid
  token_name = "terraform"
  comment    = "Used by backup clients"

  # Optional: disable the token or let it expire (Unix epoch seconds, 0 = never).
  # enable = false
  # expire = 1893456000

  # Change this value to issue a new secret in place (token and ACLs are kept).
  # Adding or removing the attribute does not regenerate the secret.
  regenerate_trigger = "2026-01"
}

# Tokens only get privileges explicitly granted to them.
resource "pbs_acl" "backup_token" {
  path    = "/datastore/backups"
  auth_id = pbs_api_token.backup.tokenid
  role    = "DatastoreBackup"
}

output "backup_token_secret" {
  value     = pbs_api_token.backup.value
  sensitive = true
}
