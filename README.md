# Notifications API

The notifications API is a central API in VIS / VSETH that manages notifications.

[TOC]

## Getting started

```bash
# Load .env.local file if exists
set -a && source .env.local && set +a

# Run server - by default only logs incoming messages - "testing first" mentality
go run main.go

# Run server with actually sending messages, but without grpc authentication.
# Handle any request without checks.
# additionally, run with highest log level
LOG_LEVEL=trace go run main.go -grpc-logging-only=false -grpc-unauthenticated
```

## Mail API

As part of its interfaces, the notification API exposes calls to send mails
