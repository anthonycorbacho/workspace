# Monorepo Makefile, run `make help` to list the targets.

GO_PKGS ?= $$(go list ./... | grep -v /vendor/)

# Services started by kit/docker-compose.yaml and their test environment, the same as the CI.
POSTGRES_PORT ?= 5432
TEST_INTEGRATION_ENV = \
	TESTINGDB_URL=postgresql://postgres:secret@localhost:$(POSTGRES_PORT)/postgres \
	TESTINGREDIS_URL=:6379 \
	TESTINGNATS_URL=nats://127.0.0.1:4222 \
	PUBSUB_EMULATOR_HOST=:8085 \
	TESTINGKAFKA_URL=127.0.0.1:19092 \
	TESTINGMQTT_URL=mqtt://127.0.0.1:1883 \
	MINIO_TEST_URL=localhost:9000

.PHONY: help onboarding lint lint-proto test test-integration bench tidy proto-gen errgen codegen rename

help:			## Show this help
		@grep -hE '^[a-z-]+:.*## ' $(MAKEFILE_LIST) | sed -e 's/:.*## /\t/'

onboarding:		## Check the tools required by the workspace
		@./sdlc/onboarding/onboarding.sh

lint:			## Run golangci-lint (see golangci.yml)
		@golangci-lint run -c golangci.yml ./...
		@cd sdlc/codegen && GOWORK=off golangci-lint run -c ../../golangci.yml ./...

lint-proto:		## Lint the protobuf files (api and kit)
		@cd api && buf lint
		@cd kit && buf lint --path pagination/v1/pagination.proto

test:			## Run the unit tests (the integration tests are skipped without their services)
		@go test -race $(GO_PKGS)
		@cd sdlc/codegen && GOWORK=off go test -race ./...

test-integration:	## Run all the tests with their services (docker compose, POSTGRES_PORT=55432 if 5432 is used)
		@POSTGRES_PORT=$(POSTGRES_PORT) docker compose -f kit/docker-compose.yaml up -d --wait
		@$(TEST_INTEGRATION_ENV) go test -race -cpu=1,4 $(GO_PKGS); \
			status=$$?; docker compose -f kit/docker-compose.yaml down; exit $$status

bench:			## Run the go benchmarks
		@go test -run='^$$' -bench=. -benchmem $(GO_PKGS)

tidy:			## Tidy the go modules and update the vendor directory
		@go mod tidy && go mod vendor
		@cd sdlc/codegen && GOWORK=off go mod tidy

proto-gen:		## Generate Go code (pb, grpc and grpc-gateway) of the api
		@cd api && buf generate
		@cd kit && buf generate --template '{"version":"v2","plugins":[{"local":["go","tool","protoc-gen-go"],"out":".","opt":"paths=source_relative"}]}' --path pagination/v1/pagination.proto

errgen:			## Generate the coded errors (kit/errors) from kit/errors/errors.defs.yaml
		@go generate ./kit/errors/...

codegen:		## Scaffold a new project in a namespace (eg: make codegen ARGS="-namespace sample -project billing")
		@GOWORK=off go -C sdlc/codegen run ./cmd/codegen -root $(CURDIR) $(ARGS)

rename:			## Rename the go module of the template (eg: make rename MODULE=github.com/acme/platform)
		@./sdlc/scripts/rename.sh $(MODULE)
