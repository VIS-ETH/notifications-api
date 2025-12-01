# Notifications API

The notifications API is a central API in VIS / VSETH that manages any kind of notification and messaging.
Its interfaces are defined by the `sip.notifications` protos, and they are exposed via gRPC.
If required, the [REST proxy](https://gitlab.ethz.ch/vseth/0403-isg/sip-api-apps/proto-restful-proxy) can be used to make it accessible via REST.

[TOC]

## Introduction

This notifications API is a standard Go project.
It closely follows the Go standard project layout & folder structure.
Each Go pkg should also include a doc comment about its functionality.
Here a quick summary about the folder structure:

- `cmd` includes the code for generated binaries (command line flags etc.)
- `internal` includes anything required for the application itself, but only useful for the application.
- `pkg` includes code that might as well be used as library for other applications.

As for exposed API functionality, the proto files and their documentation should pretty much define the entire behaviour of the API itself.

### Used codegen tools

There are a few tools that are used in order to generate (boilerplate or complex) code:

- To generate the gRPC code, we use `bufbuild`.
  It reads the proto files in [servis](servis/), which define the gRPC interface, and generates the corresponding Go code for client and server handling.
  The generated code is handling the proto encoding and the gRPC protocol, such that only the functions handling the defined RPCs must be implemented.
- Database interactions are handled by `sqlc`.
  Migrations in [sql](sql) are read by `sqlc`, as well as the queries.
  From these files, `sqlc` generates all code that interacts with the database, defines the respective types and any other code that interacts this closely to the database.

The provided `Makefile` provides a few ways to generate this code, or see the commands below in the building & compiling section.

### Functionality

The Notifications API implements a few things beyond the bare minimum for a Go project:

- It implements the gRPC health v1 protocol for its gRPC server.
  This is very useful for health checking in Kubernetes environments.
- It uses the OTEL ecosystem to expose observability information.
  It is setup to push traces and metrics to compatible collectors, provided that the correct environment variables.
  Prometheus endpoints are also exposed if simply scraping Prometheus metrics is the preferred approach
- We provide a full setup for local observability, see later section on how to use it.
- In order to be persistent and handle queued mails safely, it uses a database.

## Building, compiling & running

Building and compiling the project should be straightforward.
The Dockerfile itself of course builds the entire project from scratch, which should be very stable.
If you do not want Docker, but rather want it locally, we try to provide a few ways to compile it easily.
The following commands should help you get started

```bash
# Build, compile & run with docker compose
## This will start the docker container, listening to 6781.
## You can read the docker-compose.yaml file for details, it should be straightforward to read.
docker compose up --build
## To run the observability stack, simply run the following command.
## This will start the container as before, but also start multiple services that allow you to setup everything locally.
## Most importantly, Grafana is now available on localhost:3000
docker compose -f configs/local-observability/docker-compose.observability.yaml up --build


# Build locally
## Build locally, but codegen (which might need a few other dependencies other than Golang) via Docker
make docker=true
# Building everything, including codegen
make

# Run locally
## Useful command: load .env.local file if exists
set -a && source .env.local && set +a
## Run server - by default only logs incoming messages - "testing first" mentality
go run cmd/notifications-api/notifications-server.go
## Run server with actually sending messages, but without grpc authentication.
## Handle any request without checks.
## additionally, run with highest log level
go run cmd/notifications-api/notifications-server.go -grpc-logging-only=false -grpc-unauthenticated -log-level trace
```

## Observability  showcase infrastructure

> [!tip]
> To get a small feel for what we mean by observability, we encourage you to read our [observability docs](https://wiki.vseth.ethz.ch/x/KQCiL).

The project is setup for full observability and provides all relevant configurations to run the observability stack locally.
The commands to run everything are in the relevant building and running section, here we cover what the observability stack actually includes.

### Used components

- Grafana for visualizations and the go-to user interface.
  In the local observability docker compose setup, it is reachable on [localhost:3000](http://localhost:3000).
- Prometheus for storing metrics. It receives data from Tempo and Alloy and can scrape applications (collect metrics) by itself.
- Alloy exposes OTEL collector compatible endpoints and is essentially a swiss army knife for moving data between applications and services. Here, it simply pushes the relevant data to Tempo, Prometheus etc.
- Tempo as database to store traces.
It receives data from the OTEL interface (either directly or, like here, through Alloy), and also generates and pushes metrics to Prometheus.

### Browsing the stack

Usually, you simply want to look at Grafana (by default at [localhost:3000](http://localhost:3000)). In the sidebar, you can navigate to "Drilldown" and "Explore" and view the traces or metrics, which are usually the most interesting data anyway.

## Using the API

As part of its interfaces, the notification API exposes calls to send mails, as well as sending Slack messages.
For mails, we expect authentication via Keycloak tokens (see design explanation in the proto files).
For Slack, it suffices to send a valid slack token `xoxb-...` in an `X-Slack-Token` header over gRPC.

We provide a few ways to invoke and test the APIs below.

### Testing components individually

#### Mails and Slack only - no gRPC

Before we get to using the API directly, we first provide another way to quickly test your code.
If the goal is to test the code that is purely responsible for sending mails and slack notifications or others, you can simply call this code directly over the CLIs in the `cmd` folder.
This does not require any gRPC setup or running API etc and is probably easiest to get started.

#### gRPC code only

The API, by default, is only logging incoming requests.
This is by design, as we want to make it as easy as possible to use it in local test environments.
This is often already enough to test the gRPC specific code too, and makes it very easy to get started, also without any secrets etc.

### Invoking the API

> [!info]
> Authentication can be disabled for testing.
> You can set the correct environment variables (directly, or via .env.local), or pass the CLI flags directly.
>
> For some functionality, credentials are required (Slack notifications).
> In this case, they must be passed correctly according to the documentation in the servis proto files!

After you started the API locally via `go` or via `docker`, you can invoke the gRPC API via different tools.

#### grpcui - Web GUI

Install grpcui for example via brew, or go, or any other package manager.
Note that you will need the slack key to actually test the system.
To start it, use the following command:

```bash
cd servis
grpcui -proto sip/notifications/notifications.proto -plaintext localhost:6781
```

This will automatically open your browser and give you a GUI to interact with the API.

#### grpcurl - CLI

To test the gRPC API over a CLI (like `curl`), you can use grpcurl.
An example request / usage is left here:

```bash
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

## For administrators

Finally, here a few notes for administrators - what to do in case of errors and audit logs etc.

### Audit Logs

Audit logs _should_ be implemented by properly storing the logs of this API.
Storing all messages forever in the database is not something desirable.
However, the database does contain the last sent messages for a fixed, short period of time.

### Handling failed queued messages

> [!caution]
> Audit logs are only provided where API itself provides authentication (mails), and not where it is managed externally (Slack).
> We do expect the users to handle logs appropriately for logging purposes.

Some queuing RPC endpoints must be exposed via the proto.
These queued messages can fail, even after retries.
To avoid mishaps in these cases and endlessly retrying the same mails over and over again, we strongly recommend setting up alerts and using the observability features of this project.
The worst case is probably endlessly sending the same mail over and over again, which might happen when the API cannot update the status in the database.
The API tries to avoid this and only retries once per hour, but these are the kinds of things that must be prevented via monitoring etc.

In order to handle failures in queued mails, do not hesitate to edit the database directly (even just deleting the messages).
