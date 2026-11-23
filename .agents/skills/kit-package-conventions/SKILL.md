---
name: kit-package-conventions
description: Create and evolve kit/* packages using workspace conventions for API shape, options, errors, docs, and tests.
tags: [go, kit, foundation, package-design]
compatibility: opencode
version: 2
---

# Goal
Provide a repeatable standard for creating or updating packages under `kit/` so they stay small, stable, testable, and consistent with existing workspace patterns.

# When to use
- Creating a new package under `kit/<name>`.
- Refactoring an existing `kit/*` package API.
- Adding exported types, functions, options, or errors in `kit/*`.
- Introducing package config and runtime wiring for shared infra helpers.

# Core principles
- `kit/*` packages are shared primitives: keep business-domain logic out.
- Keep APIs minimal and stable; optimize for backwards compatibility.
- Prefer composable interfaces + small concrete implementations.
- Put cross-cutting concerns (errors, telemetry, retry, config) behind clear boundaries.
- Do not introduce new third-party libraries unless explicitly requested (remember `vendor/` is committed; run `make tidy` if dependencies change).
- Tests must cover happy path and failure modes.

# Reference patterns in this repo
- `kit/options.go` for functional options at package-level constructors.
- `kit/sql/options.go` for unexported internal `options` + exported `Option`.
- `kit/config/config.go` for YAML/env configuration decoding style.
- `kit/log/doc.go`, `kit/grpc/doc.go`, `kit/blob/doc.go`, `kit/sql/doc.go` for package docs style.
- `kit/blob/options.go`, `kit/pubsub/pubsub.go` for call-scoped option structs.
- `kit/sql/db.go`, `kit/retry/retry.go` for error wrapping and context-aware runtime helpers.

# Recommended package anatomy
For a new package `kit/<pkg>`, include only what is needed:
- `doc.go` - package comment + package declaration.
- `<pkg>.go` - core interfaces, types, and public API.
- `options.go` - functional options (`Option` or operation-specific options).
- `errors.go` - package-level error types, sentinels, or mapping helpers (if needed).
- `config.go` - configuration structs and `Open`/constructor helpers (if needed).
- `*_test.go` - unit tests; guard integration tests with env vars so `make test` passes without external services.

# API design conventions
1. Keep exported surface small.
   - Export only stable types/functions needed by consumers.
   - Keep helper structs/functions unexported unless they are part of the public contract.

2. Context and method signatures.
   - Use `context.Context` as first parameter for I/O or blocking operations.
   - Return `error` for failures; avoid panics except unrecoverable init invariants.

3. Interfaces at boundaries.
   - Define interfaces for behavior used by consumers (e.g. `Blob`, `Cache`, `Publisher`, `Queryable`).
   - Keep interfaces cohesive; avoid "god" interfaces.

4. Compatibility discipline.
   - Avoid breaking signatures/renames in shared packages.
   - Prefer additive changes (new option/function/type) over behavior-changing edits.

# Options conventions
Use one of these two patterns depending on scope:

1. Constructor/global options:
   - `type Option func(*options)` with unexported `options` struct.
   - `WithXxx(...) Option` functions for each tunable field.
   - Apply defaults first, then apply user options in order.

2. Operation-specific options:
   - `type WriteOptions struct { ... }`
   - `type WriteOption func(*WriteOptions)`
   - `WithXxx(...) WriteOption` helpers for call-level behavior.

Guidelines:
- Option names should be explicit (`WithMaxOpenConns`, `WithContentType`).
- Validate option input where possible.
- Defensively copy slices/maps passed into options.
- Avoid hidden side effects across options.

# Error conventions for kit packages
- Return `error`; wrap lower-level errors with meaningful operation context.
- Prefer `kit/errors.Wrap(...)` when wrapping errors in shared infra code.
- Define package-specific sentinel/typed errors only when callers must branch by behavior.
- Do not leak driver/protocol details unless needed by the caller contract.
- `errcheck` in `golangci.yml` enables `check-blank: true`: never silently ignore errors with `_ = ...` in non-test files without a justified `//nolint:errcheck // reason` comment.

# Documentation conventions
- Add `doc.go` with a short package comment and usage example.
- Add comments for exported types/functions where behavior is non-obvious.
- Keep comments focused on intent/contract; avoid restating obvious code.
- Update `kit/README.md` when adding a new package or Foundation option.

# Testing conventions
- Prefer table-driven tests for multi-case behavior.
- Use `testify` (`assert`/`require`) as in existing packages.
- Cover:
  - happy path,
  - invalid input,
  - dependency failure paths,
  - edge cases (nil, empty, timeout/cancel, concurrency with `-race`).
- For integration tests requiring external systems, guard with env vars (matching `Makefile` `TEST_INTEGRATION_ENV`) and skip cleanly.
- Keep unit tests deterministic and fast.

# Implementation procedure
1. Inspect nearest `kit/*` packages with similar responsibility.
2. Design public API (interfaces, constructors, options, errors).
3. Implement package files with minimal exported surface.
4. Add tests for behavior and failure modes.
5. Verify:
   - `go test -race -count=1 ./kit/<pkg>...`
   - `go test -race -count=1 ./kit/...`
   - `make lint`

# Review checklist
- Package remains domain-agnostic and reusable.
- Public API is minimal and additive (no accidental breaking changes).
- Context is used for I/O operations.
- Options follow existing repo patterns and apply defaults correctly.
- Errors are wrapped/mapped consistently and intentionally.
- Package docs and exported API docs are present.
- Tests cover happy path + failure modes under `-race`.
- No unnecessary new dependency introduced (and `make tidy` run if `go.mod` changed).
