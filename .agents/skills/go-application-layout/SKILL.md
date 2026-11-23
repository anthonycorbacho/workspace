---
name: go-application-layout
description: Define service structure and best practices using the workspace Foundation pattern and sampleapp as reference.
tags: [go, architecture, foundation, service-layout]
compatibility: opencode
version: 4
---

# Goal
Provide a repeatable project structure for Go services that keeps domain logic clean, dependencies isolated, and composition centralized. See `sdlc/documentation/go-application-layout.md` for full rationale.

# When to use
- Creating a new service (`make codegen ARGS="-namespace <ns> -project <svc>"`) or refactoring an existing one to match the Foundation pattern.
- Reviewing a service layout for adherence to domain boundaries and adapter isolation.
- Working on an existing service under `<namespace>/<service>/`.

# Core principles
- Root package contains domain types, interfaces, and business logic only.
- Subpackages are grouped by dependency and implement domain interfaces.
- A shared `mock/` subpackage provides test doubles for root interfaces.
- The `cmd/<service>/` composition root wires adapters to transports and `kit.Foundation`.
- Root packages are transport-free: no `google.golang.org/grpc`, `github.com/grpc-ecosystem`, or `api/` imports (register new service root packages in `golangci.yml` under `depguard`).

# Reference layout
```
<namespace>/<service>/
├── cmd/
│   └── <service>/
│       ├── main.go          # Composition root - config, dependency injection & bootstrap
│       ├── grpc_<res>.go    # gRPC handler wiring (transport entry point)
│       └── http_<res>.go    # Custom HTTP handler wiring (optional)
├── postgres/                # Outbound adapter implementing domain interfaces against PostgreSQL
├── inmem/                   # In-memory adapter (optional, useful for local dev/tests)
├── http/                    # Outbound adapter for HTTP APIs (optional)
├── grpc/                    # Outbound adapter for gRPC APIs (optional)
├── mock/                    # Shared mock implementations for testing
├── assets/                  # Embedded resources (SQL migrations under assets/db/, templates)
├── <domain_types>.go        # Domain entities, interfaces, and service orchestration (root package)
├── kube.yaml                # Kubernetes Deployment and Service
├── Tiltfile                 # Local Kubernetes development config
└── README.md
```

# How to apply
1. Scaffold a new service with `make codegen ARGS="-namespace <ns> -project <svc>"`.
2. Define domain types, storage/client interfaces, and service orchestration in the root package.
   - Reference: `sample/sampleapp/user.go` defines `User`, `UserStorage`, and `UserService`.
   - Keep imports limited to standard library and transport-free kit packages (`kit/errors`, `kit/id`, `kit/pagination`, OpenTelemetry).
3. Add dependency-specific adapters in subpackages.
   - Reference: `sample/sampleapp/inmem/user.go` implements `sampleapp.UserStorage`.
   - For PostgreSQL adapters, follow the `go-postgresql-layer` skill using `kit/sql`.
4. Define coded errors in `kit/errors/errors.defs.yaml` and regenerate with `make errgen`.
5. Organize embedded migrations and static assets under `assets/` (e.g. `assets/embed.go` embedding `db/*.sql`).
6. Wire dependencies in `cmd/<service>/main.go`:
   - Initialize `kit/log`, `kit.Foundation`, adapters, and transports.
   - Register readiness/liveness checks via `RegisterReadinessCheck` / `RegisterLivenessCheck`.
   - Register resource cleanup via `RegisterShutdown(func(ctx context.Context) error { ... })` so resources close in LIFO order after HTTP/gRPC drain.
   - Reference: `sample/sampleapp/cmd/sampleapp/main.go`.
7. Place transport handlers in `cmd/<service>/` with one file per entry point (e.g. `sample/sampleapp/cmd/sampleapp/grpc_user.go`).
8. Write root package tests against interfaces and mocks (`sample/sampleapp/user_test.go`).

# Best practices
- Keep the root package dependency-free; adapters import root, never the reverse.
- Use interfaces in the root package as the boundary between domain and implementation.
- Prefer small, composable adapters over mixing multiple dependencies in one package.
- Use `cmd/` as the only place where wiring and environment-specific decisions occur.
- Embed migrations and static assets in `assets/` to keep deployments self-contained.
- Keep transports thin: validate/translate protobuf or HTTP payloads and delegate to root services.
- For CRUD in root package services, prefer domain-first method signatures (`Create(ctx, u *User) error`, `Fetch(ctx, id string) (*User, error)`, `Update(ctx, u *User, masks []string) error`) over transport-style `XxxRequest`/`XxxResponse` wrappers.
- Keep `time.Time` in domain models; convert `timestamppb.Timestamp` only in transport handlers.
- Use `int` for service-level pagination/limits; convert protobuf `int32` in transport handlers.
- Prefer `len(s) == 0` over `s == ""` for empty string checks.
- Add new service root packages to `depguard` (`service-root-transport-free`) in `golangci.yml`.

# Checklist for new services
- Root package contains domain entities, interfaces, and service orchestration only.
- Root package is listed in `golangci.yml` `depguard` rules.
- Errors are defined in `kit/errors/errors.defs.yaml` and regenerated via `make errgen`.
- Adapters exist for each dependency (`postgres/`, `inmem/`, etc.) with compile-time `var _ ...` checks.
- `cmd/<service>/main.go` wires config, adapters, probes, shutdown hooks, and transports.
- `cmd/<service>/grpc_*.go` / `http_*.go` implements inbound handlers.
- `assets/` embeds migrations when using PostgreSQL.
- `make lint test` passes.
