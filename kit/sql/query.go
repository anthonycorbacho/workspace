package sql

import (
	"context"
	"database/sql"

	"github.com/jmoiron/sqlx"
)

var (
	_ Queryable = (*DB)(nil)
	_ Queryable = (*sqlx.DB)(nil)
	_ Queryable = (*sqlx.Tx)(nil)
)

// Queryable includes the context aware methods shared by DB, sqlx.DB and sqlx.Tx,
// allowing a function to run its queries either on the database or in a transaction.
//
// It is a sqlx.ExtContext, it can be used with the sqlx helpers (eg: sqlx.NamedQueryContext).
type Queryable interface {
	sqlx.ExtContext
	sqlx.PreparerContext

	GetContext(ctx context.Context, dest any, query string, args ...any) error
	SelectContext(ctx context.Context, dest any, query string, args ...any) error
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
	NamedExecContext(ctx context.Context, query string, arg any) (sql.Result, error)
	PreparexContext(ctx context.Context, query string) (*sqlx.Stmt, error)
	PrepareNamedContext(ctx context.Context, query string) (*sqlx.NamedStmt, error)
}
