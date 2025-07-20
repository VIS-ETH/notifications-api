FROM golang:1.24.5 AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
# TODO do i need to run gen_proto.sh?
RUN CGO_ENABLED=0 GOOS=linux go build -o messenger ./cmd/slackMessaging


FROM eu.gcr.io/vseth-public/base:delta AS base
# TODO Ok what is the most basic image? I feel like delta might be based on message_api
WORKDIR /app

COPY --from=builder /app/messenger .

COPY cinit.yml /etc/cinit.d/slackMessaging.yml

ENTRYPOINT [ "/app/messenger" ]