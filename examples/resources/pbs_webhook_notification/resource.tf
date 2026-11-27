variable "webhook_token" {
  type      = string
  sensitive = true
}

# Webhook target that authenticates with a secret token. Secrets are
# referenced in the body and header templates as {{ secrets.<name> }}.
resource "pbs_webhook_notification" "example" {
  name   = "ops-webhook"
  url    = "https://hooks.example.com/pbs"
  method = "post"

  headers = {
    "Content-Type"  = "application/json"
    "Authorization" = "Bearer {{ secrets.token }}"
  }

  body = jsonencode({
    title    = "{{ title }}"
    message  = "{{ message }}"
    severity = "{{ severity }}"
  })

  secret = {
    token = var.webhook_token
  }

  comment = "Notifications for the ops team"
}
