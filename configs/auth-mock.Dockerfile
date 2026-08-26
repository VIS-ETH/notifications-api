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
RUN go build cmd/auth-mock/auth-mock.go

FROM scratch

WORKDIR /

COPY --from=builder /app/auth-mock /

ENTRYPOINT [ "/auth-mock" ]
