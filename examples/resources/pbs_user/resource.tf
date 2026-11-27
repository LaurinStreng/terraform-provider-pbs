# User of the built-in pbs realm with a password
resource "pbs_user" "backup_operator" {
  userid    = "backup-operator@pbs"
  password  = var.backup_operator_password
  firstname = "Backup"
  lastname  = "Operator"
  email     = "backup-operator@example.com"
  comment   = "Managed by Terraform"
}

# Temporarily disabled user that expires on 2030-01-01
resource "pbs_user" "contractor" {
  userid  = "contractor@pbs"
  enable  = false
  expire  = 1893456000
  comment = "External contractor"
}

# PBS does not remove a user's own ACL entries when the user is deleted;
# manage them with pbs_acl so Terraform removes them together with the user.
resource "pbs_acl" "backup_operator" {
  path    = "/datastore/backups"
  auth_id = pbs_user.backup_operator.userid
  role    = "DatastoreBackup"
}

variable "backup_operator_password" {
  type      = string
  sensitive = true
}
