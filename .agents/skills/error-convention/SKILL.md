---
name: error-convention
description: Apply workspace coded error conventions across domain, adapters, and transports using kit/errors.
compatibility: opencode
version: 1
---

# Error Conventions (`kit/errors` + `ERR_XXXXX` codes)

Workspace uses transport-free coded errors defined in `kit/errors/errors.defs.yaml` and generated into `github.com/anthonycorbacho/workspace/kit/errors`. See `sdlc/documentation/application-error.md` for full design rationale.

## What You Return
- For well-defined client-facing failures (not found, already exists, invalid request, permission denied): return generated `*errors.Error` constructors from `kit/errors`.
- For unexpected/internal dependency failures: wrap with `errors.Wrap(err, "context")`. The gRPC and HTTP gateway layers log the full chain and return `errors.Internal()` (`ERR_00000`) to clients without leaking internal details.
- Avoid service-local sentinel errors for API-facing failures when a stable error code is needed.

## Source of Truth: `kit/errors/errors.defs.yaml`

All coded errors live in `kit/errors/errors.defs.yaml` and are generated via `make errgen` (`go generate ./kit/errors/...`) into:
- `kit/errors/error.gen.go` (constructors + `...Code` constants)
- `kit/errors/docs/*` (human-readable documentation)

**Never edit `kit/errors/error.gen.go` or `kit/errors/docs/*` manually.** Always update `kit/errors/errors.defs.yaml` and run `make errgen`.

## Adding New Errors

1. Find (or reserve) the domain code block in `kit/errors/errors.defs.yaml` (e.g. `00XXX` for kit, `01XXX` for sample).
2. Pick the next available `ERR_<5 digits>` code in that block. A released code is part of the API and must never change or be reused.
3. Add the error definition:

```yaml
  # Billing: 02XXX
  - name: BillingInvoiceNotFound
    code: ERR_02001
    kind: NOT_FOUND
    message: "invoice {id} not found"
    vars:
      - name: id
        type: string

  - name: BillingInvoiceAlreadyExists
    code: ERR_02002
    kind: ALREADY_EXISTS
    message: "invoice {number} already exists"
    vars:
      - name: number
        type: string
```

Notes:
- `name` becomes a Go constructor (`errors.BillingInvoiceNotFound(id)`) and code constant (`errors.BillingInvoiceNotFoundCode`).
- `kind` is transport-free (`NOT_FOUND`, `ALREADY_EXISTS`, `INVALID_ARGUMENT`, `UNAUTHENTICATED`, `PERMISSION_DENIED`, `FAILED_PRECONDITION`, `ABORTED`, `RESOURCE_EXHAUSTED`, `UNAVAILABLE`, `INTERNAL`, `UNKNOWN`, etc.). `kit/grpc` maps it to gRPC status codes and the gateway maps it to HTTP statuses.
- `message` supports `{var}` placeholders replaced by `vars` values.
- `vars` become constructor parameters and populate `Error.Details` unless `internal: true` (logged only, never sent to clients).
- `*errors.Error` is **immutable**: `WithDetail`, `WithInternal`, and `WithCause` return a new copy.

## Regenerating `kit/errors`

Run from repo root:

```bash
make errgen
```

## Using Errors by Layer

### Adapter layer (`postgres/`, `inmem/`, external clients)
Translate well-defined dependency errors to coded errors and keep the underlying error via `.WithCause(err)` (never in the client-facing message). Wrap undefined errors with `errors.Wrap`:

```go
import (
	stdsql "database/sql"

	"github.com/anthonycorbacho/workspace/kit/errors"
	"github.com/jackc/pgx/v5/pgconn"
)

func sqlWrap(err error, id, op string) error {
	if errors.Is(err, stdsql.ErrNoRows) {
		return errors.SampleUserNotFound(id).WithCause(err)
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		switch pgErr.Code {
		case "23505":
			return errors.SampleUserAlreadyExists(id).WithCause(err)
		case "23502":
			return errors.InvalidRequest().WithCause(err)
		}
	}
	return errors.Wrap(err, op)
}
```

### Root / domain layer
- Validate domain rules and return `errors.InvalidRequest().WithDetail("violations", ...)` (see `sample/sampleapp/user.go`).
- Wrap returned adapter errors with `errors.Wrap(err, op)` to add operator context; `errors.HasCode` and transport conversion unwrap automatically.
- Re-map a coded error only when domain context changes its meaning.
- Never import `google.golang.org/grpc` or `api/` in root packages (enforced by `depguard` in `golangci.yml`).

Checking coded errors:

```go
if errors.HasCode(err, errors.SampleUserNotFoundCode) {
	// handle not found
}
```

### Handler layer (`cmd/<service>/grpc_*.go`)
Return service errors as-is. `kit/grpc` converts `*errors.Error` to gRPC status + `errdetails.v1.Error` detail, turns non-coded errors into `errors.Internal()` (`ERR_00000`), and recovers panics as `ERR_00000`. For custom HTTP routes and grpc-gateway handlers, a panic before the response is committed returns `500` `ERR_00000` JSON with `trace_id` when available. A panic after commitment aborts the response.

## Style Guidelines
- Error messages start lowercase with no trailing punctuation: `unable to open file`.
- Prefer `unable to...` over `failed to...` / `can't...`.
- Use `kit/errors` (`New`, `Newf`, `Wrap`, `Wrapf`, `Is`, `As`) so callers don't need to import stdlib `errors` alongside `kit/errors`.

## Checklist
- New coded errors are added to `kit/errors/errors.defs.yaml` in the domain's reserved `ERR_XXXXX` block.
- `make errgen` is run and generated files (`error.gen.go`, `docs/*`) are committed.
- Adapters translate known dependency errors with `.WithCause(err)` and wrap unknown errors with `errors.Wrap`.
- Root packages stay transport-free (no gRPC/API imports).
- `make lint` and `make test` pass.
