---
name: go-postgresql-layer
description: Create a PostgreSQL storage adapter using kit/sql following the workspace Foundation pattern.
compatibility: opencode
version: 3
---

# PostgreSQL Application Layer

In the workspace Foundation pattern, the root package defines domain types and interfaces (dependency-free), and a `postgres/` subpackage implements those interfaces against PostgreSQL via `github.com/anthonycorbacho/workspace/kit/sql`.

## When to use
Use this skill when adding persistent PostgreSQL storage for a service under `<namespace>/<service>/`.

## Inputs
- `module`: Go module import path (default `github.com/anthonycorbacho/workspace`)
- `service`: service root package path (e.g. `sample/sampleapp`)
- `storage_interface`: root interface to implement (e.g. `sampleapp.UserStorage`)
- `resource`: primary resource name (e.g. `user`, `invoice`)
- `migration_service_name`: migrations table prefix passed to `sql.Migrate` (creates `<service>_schema_migrations`)

## Target layout
- `<service>/<resource>.go` (root package): domain types, storage interface, service
- `<service>/assets/embed.go`: embeds `db/*.sql` (and optional `seed/*.sql`)
- `<service>/assets/db/NNN_<description>.up.sql` (and `.down.sql` when needed)
- `<service>/postgres/sql.go`: PostgreSQL error mapping helper
- `<service>/postgres/<resource>.go`: storage implementation
- `<service>/postgres/<resource>_test.go`: adapter integration tests using `kit/sql.TestingDB`
- `<service>/cmd/<service>/main.go`: opens DB, runs `sql.Migrate`, registers readiness check and shutdown hook

## Conventions

### Root package (domain)
- MUST NOT import `postgres/` or transport packages (`depguard` enforces transport isolation).
- Defines domain structs and storage interfaces (e.g. `type UserStorage interface { ... }`).

### Postgres adapter (`postgres/` package)
- Use `github.com/anthonycorbacho/workspace/kit/sql` for DB access.
- Hold `kitsql.Queryable` on the storage struct so the same methods work against both `*kitsql.DB` and `*sqlx.Tx` (inside `db.WithTx`):
  - `type UserStorage struct { db kitsql.Queryable }`
  - `func New(db kitsql.Queryable) *UserStorage`
  - `var _ <root>.UserStorage = (*UserStorage)(nil)` (compile-time interface check)
- Use `github.com/jmoiron/sqlx` helpers (`sqlx.Rebind(sqlx.DOLLAR, q)`, `sqlx.NamedExecContext`) as needed.
- Observability:
  - `kit/sql` already instruments SQL queries with OpenTelemetry (`otelsqlx`).
  - Add a tracer span (`otel.Tracer("").Start(ctx, "<service>.postgres.<op>")`) when grouping multiple queries or adding domain attributes.
- SQL safety:
  - Never concatenate user input into SQL strings.
  - Parameterize all values; for dynamic queries built with `?` placeholders, finish with `q = sqlx.Rebind(sqlx.DOLLAR, q)`.
  - For list/search queries, build `where := []string{...}` + `args := []any{...}` in a small `buildListQuery(...) (string, []any)` helper.
- Transactions:
  - Prefer `db.WithTx(ctx, func(tx *sqlx.Tx) error { ... })` which handles commit, rollback on error, and rollback + re-panic on panic.
- Bulk inserts:
  - Prefer `NamedExecContext(ctx, q, entities)` with a slice of `db:"..."`-tagged structs.
- Error mapping:
  - Translate known DB errors (`stdsql.ErrNoRows`, `*pgconn.PgError` codes like `23505`, `23502`, `23503`) into generated `kit/errors` constructors with `.WithCause(err)` in `postgres/sql.go`.
  - Wrap unknown errors with `errors.Wrap(err, "<op>")` (which transports turn into `ERR_00000` Internal without leaking DB details).

## Procedure

### 1) Define the storage interface in the root package
```go
package billing

import "context"

type InvoiceStorage interface {
	Fetch(ctx context.Context, id string) (*Invoice, error)
	Create(ctx context.Context, inv *Invoice) error
	Delete(ctx context.Context, id string) error
}
```

### 2) Add migrations and embed them
Create `<service>/assets/db/001_initial.up.sql` and `<service>/assets/embed.go`:

```go
package assets

import "embed"

//go:embed db/*.sql
var DBFs embed.FS
```

`kitsql.Migrate` reads from the `db` directory by default (override with `kitsql.WithMigrationsPath("...")`).

### 3) Wire DB, migrations, probe, and shutdown in `cmd/<service>/main.go`
```go
db, err := kitsql.Open(ctx, cfg.DatabaseURL)
if err != nil {
	l.Error(ctx, "opening database", log.Error(err))
	return err
}

if err := kitsql.Migrate(ctx, cfg.DatabaseURL, "billing", assets.DBFs); err != nil {
	_ = db.Close() //nolint:errcheck // migration error is more relevant.
	l.Error(ctx, "applying database migrations", log.Error(err))
	return err
}

foundation.RegisterReadinessCheck(func(ctx context.Context) (string, error) {
	if err := db.StatusCheck(ctx); err != nil {
		return "", err
	}
	return "ok", nil
})
foundation.RegisterShutdown(func(context.Context) error {
	return db.Close()
})
```

### 4) Create `postgres/sql.go` for DB-to-domain error mapping
```go
package postgres

import (
	stdsql "database/sql"

	"github.com/anthonycorbacho/workspace/kit/errors"
	"github.com/jackc/pgx/v5/pgconn"
)

func sqlWrap(err error, id, op string) error {
	if errors.Is(err, stdsql.ErrNoRows) {
		return errors.BillingInvoiceNotFound(id).WithCause(err)
	}

	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		switch pgErr.Code {
		case "23505":
			return errors.BillingInvoiceAlreadyExists(id).WithCause(err)
		case "23502":
			return errors.InvalidRequest().WithCause(err)
		}
	}

	return errors.Wrap(err, op)
}
```

### 5) Implement the storage in `postgres/<resource>.go`
```go
package postgres

import (
	"context"

	"github.com/anthonycorbacho/workspace/<service>"
	kitsql "github.com/anthonycorbacho/workspace/kit/sql"
)

var _ <service>.InvoiceStorage = (*InvoiceStorage)(nil)

type InvoiceStorage struct {
	db kitsql.Queryable
}

func New(db kitsql.Queryable) *InvoiceStorage {
	return &InvoiceStorage{db: db}
}

func (s *InvoiceStorage) Fetch(ctx context.Context, id string) (*<service>.Invoice, error) {
	const q = `SELECT id, amount FROM invoices WHERE id = $1`

	var e invoiceEntity
	if err := s.db.GetContext(ctx, &e, q, id); err != nil {
		return nil, sqlWrap(err, id, "selecting invoice")
	}

	inv := e.toDomain()
	return &inv, nil
}
```

### 6) Add adapter tests using `kit/sql.TestingDB`
Guard integration tests when PostgreSQL is unavailable (matching `kit/sql/*_test.go` so `make test` skips cleanly without Docker, while `make test-integration` runs them):

```go
package postgres

import (
	"os"
	"testing"

	"github.com/anthonycorbacho/workspace/<service>/assets"
	kitsql "github.com/anthonycorbacho/workspace/kit/sql"
	"github.com/stretchr/testify/require"
)

func openTestDB(t *testing.T) *kitsql.DB {
	t.Helper()
	if os.Getenv("TESTINGDB_URL") == "" {
		t.Skip("TESTINGDB_URL is not set")
	}

	var tdb kitsql.TestingDB
	require.NoError(t, tdb.Open(t.Context()))
	t.Cleanup(func() { require.NoError(t, tdb.Close()) })
	require.NoError(t, kitsql.Migrate(t.Context(), tdb.DSN, "billingtest", assets.DBFs))
	return tdb.DB
}
```

## Review checklist
- Root package contains interfaces and domain types only; no imports from `postgres/`.
- `postgres/` implements root interfaces with compile-time `var _ ...` checks and accepts `kitsql.Queryable`.
- `postgres/sql.go` maps known DB errors to `kit/errors` constructors with `.WithCause(err)` and wraps unknown errors with `errors.Wrap`.
- Queries are parameterized; dynamic `?` queries are rebound via `sqlx.Rebind(sqlx.DOLLAR, q)`.
- Migrations are embedded under `assets/db/` and applied via `kitsql.Migrate` in `cmd/<service>/main.go`.
- DB readiness check and shutdown hook are registered on `Foundation`.
- Adapter tests use `kitsql.TestingDB` and skip cleanly when `TESTINGDB_URL` is unset.
- `make lint` and `make test` pass.
