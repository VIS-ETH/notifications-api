.PHONY: all
all: build

.PHONY: build
build: codegen
	go build cmd/notifications-api/notifications-server.go

.PHONY: codegen
ifeq ($(docker), true)
codegen: clean
	# Build a docker image with all dependencies generated...
	docker build . \
		--target builder \
		-t notifications-api-codegen
	
	# And copy over the files from there onto our host
	docker run \
		--entrypoint /bin/sh \
	  -v ./generated:/generated \
	  notifications-api-codegen \
		-c 'cp -R /app/generated /'
else
codegen: clean generate-protos generate-sqlc
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
