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

COPY . .

ENV CGO_ENABLED=0
RUN go build cmd/mail-sender/mail-sender.go \
  && go build cmd/notifications-api/notifications-server.go \
  && go build cmd/smtp-proxy/smtp-proxy.go

FROM gcr.io/distroless/static-debian13:nonroot

WORKDIR /

COPY --from=builder /app/notifications-server /app/smtp-proxy /app/mail-sender /
COPY sql/migrations /sql/migrations

ENV MIGRATIONS_DIR=/sql/migrations

CMD ["/notifications-server"]
