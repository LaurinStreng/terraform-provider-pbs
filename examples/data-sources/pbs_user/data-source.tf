# Read a single user
data "pbs_user" "backup" {
  userid = "backup@pbs"
}

output "backup_user_enabled" {
  value = data.pbs_user.backup.enable
}
