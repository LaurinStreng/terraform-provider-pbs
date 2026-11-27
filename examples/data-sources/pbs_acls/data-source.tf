# All ACL entries on a datastore and its namespaces
data "pbs_acls" "store1" {
  path = "/datastore/store1"
}

# Only entries on exactly this path, for one user
data "pbs_acls" "backup_user" {
  path    = "/datastore/store1"
  exact   = true
  auth_id = "backup@pbs"
}

output "backup_user_roles" {
  value = [for a in data.pbs_acls.backup_user.acls : a.role]
}
