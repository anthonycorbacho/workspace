# Workspace
[![Go Report Card](https://goreportcard.com/badge/github.com/anthonycorbacho/workspace)](https://goreportcard.com/report/github.com/anthonycorbacho/workspace)
[![go.mod Go version](https://img.shields.io/github/go-mod/go-version/anthonycorbacho/workspace)](https://github.com/anthonycorbacho/workspace)

**Workspace** is a Mono-repository template for building and deploying distributed applications.

Workspace aim to unify and structure your Go applications and deployment model. It comes with a [Kit framework](kit) for production ready Go application.
It also comes with a set a tools for managing and working with the infrastructure, the base model relies on Kubernetes via Kustomize and overlays. Workspace can run on your laptop for ease of development and can be deployed to any cloud providers.

## Contents
- [Why?](#why)
- [Documentation](#documentation)
- [Requirements](#requirements)
- [Installation](#installation-and-usage)
- [CI/CD](#cicd)

## Why?
Mono repo is a model that aims to group a set of services, tool, and deployment into a single repository, by doing so you can have this benefit “out of the box”;

 - Unify versioning, one source of truth
 - Atomic change
 - Unified deployment model for all applications
 - Enforced tooling (linter, build, code search, etc)
 - Extensive code sharing and reuse
 - Simplified dependency management
 - Large-scale refactoring. codebase modernization
 - Collaboration across teams
 - Flexible team boundaries and code ownership
 - Code visibility and clear tree structure providing implicit team namespacing

### The Model
The proposed approach for this mono repo architecture is to follow the `namespace your applications per domain` strategy.
The good part of namespacing our application per domain is that you can structure the code directory to also reflect our Kubernetes namespacing strategies, by doing so you will have a logical and visual representation for the infrastructure and code. This will improve the debugging and conceptualization of the microservice architecture.
Another benefit of namespacing the applications is that you will be able to also apply specific resource limits and service account rules per namespace, limiting the resource allocation and visibility of our microservices.

```bash
workspace/
├── .github                       # GitHub folder that contain workflow, codeowners and templates
├── README.md
├── Tiltfile                      # Configuration for running a local env
├── go.mod
├── go.sum
├── api                           # Represent our Proto API definition
├── kit                           # Represent the Go framework for building services
├── sample                        # Represent a sample namespace and service
│   ├── sampleapp
│   ├── kustomization.yaml
│   ├── namespace.yaml
│   ├── resource-limit.yaml
│   └── service-account.yaml 
├── sdlc                          # Represent a software development tools
├── overlays                      # Represent a deployment mode (local, staging, production)
│   └── local
│       ├── kustomization.yaml
│       └── dependencie           # Represent a dependency configuration
├── vendors                       # Represent all the vendor files
├── .ko.yaml                      # Ko build settings
├── LICENSE                       # Workspace licensing
├── Makefile                      # Simple Makefile for short-cut. run command `make help`
├── golangci.yml                  #  List of linting rules
└── tools.go
```

### Observability
Workspace is built with observability at heat. Kit integrate with OpenTelemetry and local overlays set up the basic Loki, Grafana, Tempo, Prometheus stack.
All tracing, login and monitoring endpoint and setting are preset via Kit framework.

## Documentation
In order to have more context on the decision and guidelines, you can refer to the [documentation folder](sdlc/documentation/README.md).

## Requirements
You need to have Go installed in your system.
The minimal version of Go is `1.20`

You can run `make onboarding` to see if you have required tools missing.

- [Rancher-desktop](https://docs.rancherdesktop.io/getting-started/installation/) or [Docker-desktop](https://www.docker.com/products/docker-desktop/)
- [Go](https://go.dev/dl/) `brew install go`
- [ko](https://github.com/google/ko) `brew install ko`
- [Kustomize](https://kubectl.docs.kubernetes.io/installation/kustomize/) `brew install kustomize`
- [helm](https://helm.sh/docs/intro/install/) `brew install helm`
- [vendir](https://carvel.dev/vendir/docs/v0.32.0/install/) `brew tap vmware-tanzu/carvel; brew install vendir`
- [Tilt](https://tilt.dev/) `brew install tilt`
- [Buf](https://docs.buf.build/installation) and [Generate code](https://docs.buf.build/tour/generate-go-code) `brew install buf` with following binaries
  - go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
  - go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest
  - go install github.com/envoyproxy/protoc-gen-validate@latest
  - go install github.com/grpc-ecosystem/grpc-gateway/v2/protoc-gen-grpc-gateway@latest
  - go install github.com/grpc-ecosystem/grpc-gateway/v2/protoc-gen-openapiv2@latest
- [golangci-lint](https://github.com/golangci/golangci-lint) `brew install golangci-lint`
- [kubeseal](https://github.com/bitnami-labs/sealed-secrets) `brew install kubeseal`
- [mockery](https://github.com/vektra/mockery) `brew install mockery`

## Installation and usage
This workspace is built using ko, Tilt and Go modules.

```bash
# First fork or clone the repository
git clone git@github.com:anthonycorbacho/workspace.git

# you can run the whole project on your local kubernetes (via docker desktop or rancher desktop)
tilt up
```

Workspace comes with a [simple app](sample/sampleapp) that illustrate how to use this framework.

### Accessible Dashboard
 - Tilt dashboard http://localhost:10350/
 - Grafana dashboard http://localhost:8090/?orgId=1

## CI/CD
GitHub Actions are used to check codestyle via golangci (you can check golangci.yml for more details).

### CI
We are using GitHub actions for building the OCI images and push it to the OCI repository.
The pipeline is trigger once a Pull Request is merged into the main branch.

By default, the push will happen on the [default GitHub Package](https://github.com/anthonycorbacho?tab=packages). You can choose to override where you want your image to be pushed.

### CD
It is strongly recommended to set up ArgoCD for managing the deployment.

TODO: document/argoCD 
