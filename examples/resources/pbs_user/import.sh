# Users are imported by their userid (name@realm).
# The password is not imported; PBS never returns it.
terraform import pbs_user.backup_operator backup-operator@pbs
