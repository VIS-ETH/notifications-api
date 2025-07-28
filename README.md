# 2message2api

The successor to message_api, which (for now) only includes slack messages.
We are waiting for the vseth message api for a better mail service.

## Setup and testing

Install grpcui for example via brew. Use this to test any grpc request locally. Note that you will need the slack key to actually test the system. 

Add the key value pair to the request metadata: {
    name: authorization,
    value: insert-message-key
}

Define both the slack and message api key in your .env.local file as follows:
RUNTIME_SLACK_API_KEY=u-actually-need-the-real-one-here
RUNTIME_MESSAGE_API_KEY=define-how-u-want-it-so-it-is-the-same-as-in-authorization

From here, run `docker compose up --build` and the following at the root of the project when docker is ready:

```
 grpcui -plaintext -proto servis/self/messaging.proto localhost:6781
```