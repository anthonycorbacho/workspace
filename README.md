# Workspace

**Workspace** is a Monorepo template that will help you to define and work with Go cloud native application.

It was created to scratch my itch while working with hundreds of different repositories. Workspace aim to unify and structure your Go applications and deployment model.
It comes with a `Kit` framework for Go application that is a boilerplate templating for production ready application out of the box.

## Contents
- [Why?](#why)
- [Requirements](#requirements)
- [Installation](#installation)
- [CI/CD](#cicd)
- [Tooling](#tooling)

## Why?
Mono repo is a model that aims to group a set of services into a single repository, by doing so we can have this benefit “out of the box”;

 - Unify versioning, one source of truth 
 - Extensive code sharing and reuse 
 - Simplified dependency management 
 - Atomic change 
 - Large-scale refactoring. codebase modernization 
 - Collaboration across teams 
 - Flexible team boundaries and code ownership 
 - Code visibility and clear tree structure providing implicit team namespacing 
 - Enforced tooling (linter, build, code search, etc)

### The Model
The proposed approach for this mono repo architecture is to follow the `namespace your applications per domain` strategy.
The good part of namespacing our application per domain is that you can structure the code directory to also reflect our Kubernetes namespacing strategies, by doing so you will have a logical and visual representation for the infrastructure and code. This will improve the debugging and conceptualization of the microservice architecture.
Another benefit of namespacing the applications is that you will be able to also apply specific resource limits and service account rules per namespace, limiting the resource allocation and visibility of our microservices.

```bash
cloud-workspace/
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

## Requirements
You need to have Go installed in your system.
The minimal version of Go is `1.20`

You can run `make onboarding` to see if you have required tools missing.

- [Rancher-desktop](https://docs.rancherdesktop.io/getting-started/installation/)
- [Tilt](https://tilt.dev/)
- [Go](https://go.dev/dl/)
- [Kustomize](https://kubectl.docs.kubernetes.io/installation/kustomize/)
- [Buf](https://docs.buf.build/installation) and [Generate code](https://docs.buf.build/tour/generate-go-code)
- [golangci-lint](https://github.com/golangci/golangci-lint) `brew install golangci-lint`
- [kubeseal](https://github.com/bitnami-labs/sealed-secrets) `brew install kubeseal`
- [mockery](https://github.com/vektra/mockery) `brew install mockery`
- [ko](https://github.com/google/ko) `brew install ko` for tilt go container image

## Installation

This workspace is build using ko, Tilt and Go modules. Here is the exhaustive list that we will mainly use.

```bash
# First clone the repository
git clone git@github.com:anthonycorbacho/workspace.git

# From there you can run all the projects with:
tilt up
```

### Accessible Dashboard
 - Tilt dashboard http://localhost:10350/
 - Grafana dashboard http://localhost:8090/?orgId=1

## CI/CD

TBD

## Tooling
Makefile that will help you to trigger/run some actions

```bash
$ make help
linter:			 Run linter
proto-gen:		 Generate Go code (pb, grpc and grpc-gateway) of our api
test:			 Run unit test (without integration tests)
bench:			 Run go benchmarks
onboarding:      Install tools for the project
codegen:		 install the codegen: `make codegen && codegen`
help:			 Show this help
```
