# AGENTS.md

Development guide for AI coding agents working in this Go monorepo (`github.com/anthonycorbacho/workspace`).

## Overview

Workspace is a Go `1.27+` monorepo template for building and deploying distributed gRPC + HTTP services on Kubernetes:

- `kit/` — Reusable service framework (`kit.Foundation`) and infrastructure packages (`blob`, `cache`, `config`, `errors`, `grpc`, `id`, `log`, `namesgenerator`, `pagination`, `password`, `pubsub`, `retry`, `sql`, `telemetry`, `validator`, `workerpool`).
- `api/` — Protobuf definitions (`buf`, Protovalidate, `grpc-gateway`, OpenAPI v2).
- `<namespace>/<service>/` — Domain-namespaced services (see `sample/sampleapp/` as reference).
- `overlays/` — Kustomize deployment overlays (`local`, `local-foundation` Grafana LGTM stack).
- `sdlc/` — Tooling (`codegen`, `errgen`, `onboarding`, `scripts/rename.sh`) and architecture docs (`sdlc/documentation/`).
- `vendor/` — Committed Go module dependencies.

## Commands

Run from the repository root (`Makefile`):

- `make onboarding` — Check required local tools (`go`, `docker`, `kubectl`, `helm`, `tilt`, `ko`, `buf`, `golangci-lint`).
- `make lint` — Run `golangci-lint` (root module and `sdlc/codegen`).
- `make lint-proto` — Run `buf lint` on `api/` and `kit/pagination/v1`.
- `make test` — Run unit tests with `-race` (integration tests skip cleanly without backing services).
- `make test-integration` — Start `kit/docker-compose.yaml` services and run all tests with `-race -cpu=1,4`.
- `make bench` — Run Go benchmarks with `-benchmem`.
- `make proto-gen` — Regenerate protobuf, gRPC, grpc-gateway, and OpenAPI code (`buf` + `go tool` plugins).
- `make errgen` — Regenerate `kit/errors/error.gen.go` and `kit/errors/docs/*` from `kit/errors/errors.defs.yaml`.
- `make tidy` — Run `go mod tidy && go mod vendor` (and tidy `sdlc/codegen`).
- `make codegen ARGS="-namespace <ns> -project <svc>"` — Scaffold a new service.
- `make rename MODULE=<new-module-path>` — Rename the Go module across code, protos, configs, and docs.

## Architecture & Conventions

### Foundation Pattern (`kit/`)
Services are bootstrapped with `kit.NewFoundation(name, opts...)`:
- **Servers**: HTTP/grpc-gateway (`:8080`, `FOUNDATION_HTTP_ADDRESS`), gRPC (`:8081`, `FOUNDATION_GRPC_ADDRESS`), Prometheus metrics (`:9090`, `FOUNDATION_METRICS_ADDRESS`), Kubernetes probes & `/debug/pprof` (`:9091`, `FOUNDATION_PROBES_ADDRESS`).
- **Lifecycle**:
  - Register gRPC services (`RegisterService`), gateway handlers (`RegisterServiceHandler`), and custom HTTP routes (`RegisterHTTPHandler`).
  - Register context-aware probes (`RegisterLivenessCheck`, `RegisterReadinessCheck`).
  - Register cleanup hooks (`RegisterShutdown`) — executed in LIFO order after HTTP and gRPC drain.
  - Run with `Serve()` or `ServeContext(ctx)`.
  - Configure `WithShutdownDelay` / `FOUNDATION_SHUTDOWN_DELAY` for zero-downtime Kubernetes rolling deploys (readiness flips to `503` while public listeners wait before draining).
- **Hardening**:
  - HTTP panic recovery wraps both custom routes and grpc-gateway routes, logging with trace correlation and returning `500` `ERR_00000` JSON with `trace_id`.
  - Default HTTP timeouts (`ReadTimeout: 15s`, `WriteTimeout: 15s`, `ReadHeaderTimeout: 5s`, `IdleTimeout: 60s`, `MaxHeaderBytes: 1 MiB`) and optional `WithGatewayAllowedHeaders`.

### Service Layout (`sdlc/documentation/go-application-layout.md`)
Every service separates domain logic from infrastructure:
- **Root package** (`<namespace>/<service>/`): domain types, interfaces, and orchestration only. Must stay transport-free (no `google.golang.org/grpc`, `github.com/grpc-ecosystem`, or `api/` imports — enforced by `depguard` in `golangci.yml`).
- **Adapters** (`postgres/`, `inmem/`, `http/`, `grpc/`): implement root interfaces with compile-time `var _ ...` checks.
- **Mocks** (`mock/`): test doubles for root interfaces.
- **Assets** (`assets/`): embedded SQL migrations (`assets/db/*.sql`) and static files.
- **Composition root** (`cmd/<service>/`): `main.go` wires config, adapters, probes, shutdown hooks, and `Foundation`; inbound handlers live alongside it (`grpc_*.go`, `http_*.go`).

### Coded Errors (`sdlc/documentation/application-error.md`)
- Define client-facing errors in `kit/errors/errors.defs.yaml` (`ERR_<5 digits>` per domain block) and run `make errgen`. Never edit generated error files by hand.
- `*errors.Error` is immutable: `WithDetail`, `WithInternal`, and `WithCause` return copies.
- Adapters translate known dependency errors (`sql.ErrNoRows`, `*pgconn.PgError`) to coded errors with `.WithCause(err)` and wrap unknown errors with `errors.Wrap`.
- Root packages wrap errors for context and validate domain rules (`errors.InvalidRequest()`).
- Handlers return errors as-is; `kit/grpc` maps coded errors onto the wire and converts unknown errors to `ERR_00000`.

### Observability
- **Tracing & Metrics**: OpenTelemetry (`OTEL_EXPORTER_OTLP_ENDPOINT`) + Prometheus (`/metrics`).
- **Logging**: `kit/log` provides structured JSON logging backed by stdlib `log/slog` with trace correlation (`TraceId`, `SpanId`, `TraceFlags`), caller paths, stacktraces on `Error`/`Fatal`, and bounded sampling. Keep `slog` behind `kit/log`.

### Linting & Code Quality (`golangci.yml`)
- `errcheck` has `check-blank: true`: handle every error; any intentional ignore in non-test code requires an explicit `//nolint:errcheck // reason` comment.
- `depguard` enforces transport isolation for `kit/errors` and service root packages (add new service root packages to `golangci.yml`).
- Keep cyclomatic complexity `<= 16` (`gocyclo`), line length `<= 200` (`lll`), and code formatted with `gofmt`/`goimports`.
- Do not introduce third-party dependencies unless requested; run `make tidy` whenever `go.mod` changes so `vendor/` stays in sync.
- Write table-driven unit tests with `testify` (`assert`/`require`) covering both happy paths and failure modes, and verify with `make lint` and `make test`.

## Skills

Detailed conventions and workflows live in `.agents/skills/`:
- `api-convention` — Protobuf, AIP naming, `grpc-gateway`, Protovalidate.
- `error-convention` — `kit/errors` catalog, `make errgen`, layered error translation.
- `go-application-layout` — Service package layout and `Foundation` wiring.
- `go-postgresql-layer` — PostgreSQL adapters using `kit/sql`, migrations, and `TestingDB`.
- `kit-package-conventions` — Designing and evolving `kit/*` packages.
- `go-naming`, `go-packages`, `go-functional-options`, `go-defensive`, `go-data-structures` — Go style and defensive patterns.
- `postgres` — PostgreSQL schema, indexing, MVCC, WAL, and query optimization.
