# ACL entries are imported by "<path>:<auth_id>:<role>".
terraform import pbs_acl.backup_user "/datastore/store1:backup@pbs:DatastoreBackup"
terraform import pbs_acl.backup_token "/datastore/store1/clients:backup@pbs!client:DatastoreReader"
