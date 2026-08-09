FROM golang:1.26-alpine AS builder

RUN apk add git npm && npm install -g @bufbuild/buf@1.71.0

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY servis servis
COPY buf.gen.yaml buf.yaml ./
RUN buf generate .

COPY sqlc.yaml ./
COPY sql sql
RUN go tool sqlc generate

COPY cmd cmd
COPY pkg pkg
COPY internal internal

RUN go build cmd/notifications-api/notifications-server.go

FROM gcr.io/distroless/base

COPY --from=builder /app/notifications-server /
COPY sql/migrations /sql/migrations

ENV MIGRATIONS_DIR=/sql/migrations

ENTRYPOINT ["/notifications-server"]
