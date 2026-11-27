# List all users
data "pbs_users" "all" {}

# List only users of the built-in "pbs" realm
data "pbs_users" "pbs" {
  realm = "pbs"
}

output "pbs_user_ids" {
  value = [for u in data.pbs_users.pbs.users : u.userid]
}
