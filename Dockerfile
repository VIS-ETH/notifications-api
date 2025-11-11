FROM golang:1.25.4-alpine AS builder

RUN apk add git \
  && go install google.golang.org/protobuf/cmd/protoc-gen-go@v1.36.9 \
  && go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@v1.5.1 \
  && apk add npm && npm install -g @bufbuild/buf
#&& go install github.com/sqlc-dev/sqlc/cmd/sqlc@v1.30.0


WORKDIR /app

COPY servis servis
COPY buf.gen.yaml buf.yaml ./
RUN buf generate .


#COPY sqlc.yaml ./
#COPY sql sql
#RUN sqlc generate

COPY go.mod go.sum ./
RUN go mod download

COPY internal internal
COPY main.go ./

RUN go build .

FROM gcr.io/distroless/base

COPY --from=builder /app/notifications-api /
#COPY sql/migrations /sql/migrations

ENTRYPOINT ["/notifications-api"]
