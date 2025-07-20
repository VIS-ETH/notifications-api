#!/usr/bin/env bash
protoc \
  --go_out=internal/pb --go_opt=paths=source_relative \
  --go-grpc_out=internal/pb --go-grpc_opt=paths=source_relative \
  servis/self/messaging.proto