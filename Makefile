# Monorepo Makefile.

# Default Go binary.
ifndef GOROOT
  GOROOT = /usr/local/go
endif

# Determine the OS to build.
ifeq ($(OS),)
  ifeq ($(shell  uname -s), Darwin)
    GOOS = darwin
  else
    GOOS = linux
  endif
else
  GOOS = $(OS)
endif

GOCMD = GOOS=$(GOOS) go
GOTEST = $(GOCMD) test -race
GO_PKGS?=$$(go list ./... | grep -v /vendor/)

# See golangci.yml for linters setup
linter:			## Run linter
		@golangci-lint run -c golangci.yml ./...;


proto-gen:		## Generate Go code (pb, grpc and grpc-gateway) of the api
		@cd api && buf generate;

test:			## Run unit test (without integration tests)
		$(GOTEST) -v $(GO_PKGS)

bench:			## Run go benchmarks
		$(GOCMD) test -tags integration -bench=. ./... -benchmem

onboarding:		## Install tools for the project
		@./sdlc/onboarding/onboarding.sh

codegen:		## install codegen in the gopath
		@cd sdlc/codegen && $(GOCMD) install cmd/codegen.go

help:			## Show this help
		@fgrep -h "##" $(MAKEFILE_LIST) | fgrep -v fgrep | sed -e 's/\\$$//' | sed -e 's/##//'
