# Workspace
[![Go Report Card](https://goreportcard.com/badge/github.com/anthonycorbacho/workspace)](https://goreportcard.com/report/github.com/anthonycorbacho/workspace)
[![go.mod Go version](https://img.shields.io/github/go-mod/go-version/anthonycorbacho/workspace)](https://github.com/anthonycorbacho/workspace)

**Workspace** is a Mono-repository template for building and deploying distributed applications.

Workspace aims to unify and structure your Go applications and their deployment model. It comes with a [Kit framework](kit) for production ready Go applications.
It also comes with a set of tools for managing and working with the infrastructure, the base model relies on Kubernetes via Kustomize and overlays. Workspace runs on your laptop for ease of development and can be deployed to any cloud provider.

## Contents
- [Why?](#why)
- [Documentation](#documentation)
- [Requirements](#requirements)
- [Installation](#installation-and-usage)
- [CI/CD](#cicd)

## Why?
Mono repo is a model that aims to group a set of services, tools, and deployments into a single repository, by doing so you get these benefits “out of the box”:

 - Unify versioning, one source of truth
 - Atomic change
 - Unified deployment model for all applications
 - Enforced tooling (linter, build, code search, etc)
 - Extensive code sharing and reuse
 - Simplified dependency management
 - Large-scale refactoring, codebase modernization
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
│   └── service-account.yaml
├── sdlc                          # Represent a software development tools
│   ├── codegen                   # Scaffold a new project (make codegen)
│   ├── documentation             # Decisions and guidelines
│   ├── errgen                    # Generate the coded errors (make errgen)
│   ├── onboarding                # Check the required tools (make onboarding)
│   └── scripts                   # Rename the template module (make rename)
├── overlays                      # Represent a deployment mode (local, staging, production)
│   ├── local                     # Namespaces deployed locally
│   └── local-foundation          # Local dependencies (monitoring stack)
├── vendor                        # Represent all the vendor files
├── .ko.yaml                      # Ko build settings
├── LICENSE                       # Workspace licensing
├── Makefile                      # Simple Makefile for short-cut. run command `make help`
└── golangci.yml                  # List of linting rules
```

### Observability
Workspace is built with observability at heart. The Kit integrates with OpenTelemetry and Prometheus, and the
[local-foundation overlay](overlays/local-foundation) deploys the Grafana LGTM stack: Alloy (collector), Loki (logs),
Tempo (traces), Prometheus (metrics) and Pyroscope (profiles).
The tracing, logging and metrics of a service built with the Kit (`kit.Foundation`) are set up out of the box: one
trace follows a request from the HTTP gateway to the gRPC handler and its downstream calls.

## Documentation
In order to have more context on the decisions and guidelines, you can refer to the [documentation folder](sdlc/documentation/README.md).

## Requirements
Run `make onboarding` to check the required tools are installed.

- [Go](https://go.dev/dl/) `1.27` or newer, `brew install go`
- [Docker](https://www.docker.com/products/docker-desktop/) (or [Rancher Desktop](https://docs.rancherdesktop.io/getting-started/installation/)) with Docker Compose and a local Kubernetes cluster
- [kubectl](https://kubernetes.io/docs/tasks/tools/) `brew install kubectl`
- [helm](https://helm.sh/docs/intro/install/) `brew install helm`, used by kustomize to render the monitoring charts
- [Tilt](https://tilt.dev/) `brew install tilt`
- [ko](https://github.com/ko-build/ko) `brew install ko`
- [Buf](https://buf.build/docs/installation) `brew install buf`, the protoc plugins are pinned in `go.mod` (`tool` directive) and run with `go tool`, no install needed
- [golangci-lint](https://github.com/golangci/golangci-lint) `brew install golangci-lint`

Optional tools:
- [vendir](https://carvel.dev/vendir/docs/latest/install/) `brew tap carvel-dev/carvel; brew install vendir`, to update the vendored helm charts
- [mockery](https://github.com/vektra/mockery) `brew install mockery`, to regenerate the mocks

## Installation and usage
This workspace is built using ko, Tilt and Go modules.

```bash
# Create a repository from this template, or clone it
git clone git@github.com:anthonycorbacho/workspace.git
cd workspace

# Rename the Go module (go.mod, imports, protos, CODEOWNERS...)
make rename MODULE=github.com/acme/platform

# Run the whole project on your local kubernetes (docker desktop or rancher desktop)
tilt up
```

Workspace comes with a [sample app](sample/sampleapp) that illustrates how to use the framework, `make help` lists the
development commands (lint, tests, code generation...).

### Create a new service
`make codegen` scaffolds a new project in a namespace (kubernetes files, minimal main, Tiltfile and CI workflows),
see [sdlc/README.md](sdlc/README.md#codegen).

```bash
make codegen ARGS="-namespace sample -project billing"
```

### Ports
A service built with the Kit listens on:

| Port | Purpose                                   | Environment variable          |
|------|-------------------------------------------|-------------------------------|
| 8080 | HTTP: grpc-gateway and custom handlers    | `FOUNDATION_HTTP_ADDRESS`     |
| 8081 | gRPC                                      | `FOUNDATION_GRPC_ADDRESS`     |
| 9090 | Prometheus metrics (`/metrics`)           | `FOUNDATION_METRICS_ADDRESS`  |
| 9091 | Kubernetes probes (`/healthz`, `/readyz`) | `FOUNDATION_PROBES_ADDRESS`   |

Locally, Tilt forwards:

| URL                    | Purpose                                         |
|------------------------|-------------------------------------------------|
| http://localhost:10350 | Tilt dashboard                                  |
| http://localhost:8090  | Grafana (logs, traces, metrics and profiles)    |
| http://localhost:8091  | Alloy UI                                        |
| http://localhost:8010  | sample/sampleapp HTTP API                       |
| http://localhost:8011  | sample/sampleapp metrics                        |

## CI/CD
GitHub Actions check every pull request: golangci-lint (see `golangci.yml`), `go.mod` and `vendor` are up to date,
`buf lint` and `buf breaking`, the generated code (protos, coded errors) is up to date, and the tests of the changed
projects run (the kit tests run with their services started by docker compose).

### CI
The OCI images are built with ko and pushed once a pull request is merged into the main branch, tagged `latest` and
with the commit SHA.

By default, the images are pushed to the [GitHub Packages](https://github.com/anthonycorbacho?tab=packages) of the
repository owner, see `.github/workflows/ko-builder.yaml` to push them somewhere else.

### CD
The deployment is not part of the template: the overlays can be applied by any GitOps tool, eg:
[Argo CD](https://argo-cd.readthedocs.io/) or [Flux](https://fluxcd.io/), with one overlay per environment.
