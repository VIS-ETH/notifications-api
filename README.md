# Notifications API

The notifications API is a central API in VIS / VSETH that manages notifications.

[TOC]

## Getting started

```bash
# Building everything
make
# Building everything, but generate protos via Docker
make docker=true

# Load .env.local file if exists
set -a && source .env.local && set +a

# Run server - by default only logs incoming messages - "testing first" mentality
go run cmd/notifications-api/notifications-server.go

# Start API via docker compose
docker compose up --build
# Start API via docker compose with full observability stack
docker compose -f configs/local-observability/docker-compose.observability.yaml up --build

# Run server with actually sending messages, but without grpc authentication.
# Handle any request without checks.
# additionally, run with highest log level
go run cmd/notifications-api/notifications-server.go -grpc-logging-only=false -grpc-unauthenticated -log-level trace
```

## Sending mails

As part of its interfaces, the notification API exposes calls to send mails.

## Testing the API

Install grpcui for example via brew. Use this to test any grpc request locally. Note that you will need the slack key to actually test the system.

Add the key value pair to the request metadata:

```
{
    name: authorization,
    value: insert-message-key
}
```

```
cd servis
grpcui -proto sip/notifications/notifications.proto -plaintext localhost:6781
```

Or via long request

```
cd servis
grpcurl -plaintext -proto sip/notifications/mail.proto -d '{
  "replyTo": [
    {
      "mailAddress": {
        "name": "Test ReplyTo",
        "address": "test@vis.ethz.ch"
      }
    }
  ],
  "to": [
    {
      "mailAddress": {
        "name": "User1",
        "address": "test-user-1@vis.ethz.ch"
      }
    }
  ],
  "cc": [
    {
      "mailAddress": {
        "name": "User1",
        "address": "test-user-1@math.ethz.ch"
      }
    },
    {
      "mailAddress": {
        "name": "User2",
        "address": "test-user-2@inf.ethz.ch"
      }
    }
  ],
  "bcc": [
    {
      "mailAddress": {
        "name": "Secret User",
        "address": "secret@ethz.ch"
      }
    }
  ],
  "extraHeader": [],
  "from": {
    "mailAddress": {
      "name": "Test Sender",
      "address": "test-noreply@vis.ethz.ch"
    }
  },
  "subject": "Test Email",
  "plainText": "Hey student,\n\nAbort mission and return back to studies...\n\nPlease....\n\n\n\n\nWarm regards,\nYour predecessors"
}' localhost:6781 sip.notifications.MailService/SendMail
```
