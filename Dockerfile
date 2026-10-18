FROM golang:1.27-alpine AS build-codegen

# hadolint ignore=DL3018
RUN apk add --no-cache git npm && npm install -g @bufbuild/buf@1.74.0

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY servis servis
COPY buf.gen.yaml buf.yaml ./
RUN buf generate .

COPY sqlc.yaml ./
COPY sql sql
RUN go tool sqlc generate

FROM build-codegen AS build-codegen-all
# Requires code to generate all mock interfaces
COPY . .
RUN go tool mockery

FROM build-codegen-all AS tests

FROM golang:1.27-alpine AS codegen
WORKDIR /app
COPY --from=build-codegen-all /app/generated /app/generated

FROM build-codegen AS builder

COPY . .
ENV CGO_ENABLED=0

FROM builder AS auth-mock-builder
RUN go build cmd/auth-mock/auth-mock.go
FROM scratch AS auth-mock
WORKDIR /
COPY --from=auth-mock-builder /app/auth-mock /
ENTRYPOINT [ "/auth-mock" ]

FROM builder AS bundled-builder

RUN go build cmd/bundled/bundled.go

FROM gcr.io/distroless/static-debian13:nonroot

WORKDIR /

COPY --from=bundled-builder /app/bundled /notifications-api
COPY sql/migrations /sql/migrations

ENV MIGRATIONS_DIR=/sql/migrations

ENTRYPOINT ["/notifications-api"]
