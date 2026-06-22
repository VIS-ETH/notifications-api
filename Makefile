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

.PHONY: clean_sql clean_pb clean
clean_sql:
	rm -rf generated/sql

clean_pb:
	rm -rf generated/pb

clean:
	rm -rf generated

.PHONY: generate-sqlc
generate-sqlc: clean_sql
	go tool sqlc generate

.PHONY: generate-protos
generate-protos: clean_pb
	npm exec @bufbuild/buf generate
