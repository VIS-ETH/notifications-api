.PHONY: all
all: build

.PHONY: build
build: codegen
	go build .

.PHONY: codegen
ifeq ($(docker), true)
codegen: clean
	# Build a docker image with all dependencies generated...
	docker build backend \
		--target builder \
		-t mail-api-codegen
	
	# And copy over the files from there onto our host
	docker run \
		--entrypoint /bin/sh \
	  -v ./generated:/generated \
	  mail-api-codegen \
		-c 'cp -R /app/generated /'
else
codegen: clean generate-protos # generate-sqlc
endif

.PHONY: clean
clean:
	rm -rf generated

#.PHONY: generate-sqlc
#generate-sqlc: clean
#	cd backend && sqlc generate

.PHONY: generate-protos
generate-protos: clean
	npx @bufbuild/buf generate
