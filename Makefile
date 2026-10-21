.PHONY: all
all: build test

.PHONY: build
build: codegen
	go build cmd/notifications-api/notifications-server.go

.PHONY: test test-coverage
test: codegen
	go test ./...

test-coverage: codegen
	go test -coverpkg=./... -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out
	go tool cover -func=coverage.out

.PHONY: codegen
ifeq ($(docker), true)
codegen: clean
	docker compose -f configs/compose.codegen.yml up --build
else
codegen: clean generate-protos generate-sqlc generate-mockery
endif

.PHONY: clean-sql clean-pb clean-mockery clean
clean-sql:
	rm -rf generated/sql

clean-pb:
	rm -rf generated/pb

clean-mockery:
	rm -rf generated/mockery

clean:
	rm -rf generated

.PHONY: generate-sqlc
generate-sqlc: clean-sql
	go tool sqlc generate

.PHONY: generate-protos
generate-protos: clean-pb
	npm exec @bufbuild/buf generate

.PHONY: generate-mockery
generate-mockery: clean-mockery generate-sqlc
	go tool mockery
