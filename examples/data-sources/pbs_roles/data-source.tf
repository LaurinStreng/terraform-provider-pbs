# List the built-in roles and their privileges
data "pbs_roles" "all" {}

output "datastore_backup_privileges" {
  value = one([for r in data.pbs_roles.all.roles : r.privileges if r.role_id == "DatastoreBackup"])
}
