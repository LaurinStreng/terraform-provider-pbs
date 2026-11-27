# API tokens are imported by their full ID "user@realm!tokenname".
# The secret cannot be read back, so `value` is null after import.
# Adding `regenerate_trigger` to the configuration afterwards only records its
# value and keeps the current secret; change it again to issue a new secret
# (stored in `value`). Removing the trigger never regenerates the secret.
terraform import pbs_api_token.backup 'backup@pbs!terraform'
