package sql

import (
	"context"
	"database/sql"

	"github.com/jmoiron/sqlx"
)

var _ Queryable = (*sqlx.DB)(nil)
var _ Queryable = (*sqlx.Tx)(nil)

// Queryable includes all methods shared by sqlx.DB and sqlx.Tx, allowing
// either type to be used interchangeably.
type Queryable interface {
	sqlx.Ext
	sqlx.ExecerContext
	sqlx.PreparerContext
	sqlx.QueryerContext
	sqlx.Preparer

	GetContext(context.Context, any, string, ...any) error
	SelectContext(context.Context, any, string, ...any) error
	Get(any, string, ...any) error
	MustExecContext(context.Context, string, ...any) sql.Result
	PreparexContext(context.Context, string) (*sqlx.Stmt, error)
	QueryRowContext(context.Context, string, ...any) *sql.Row
	Select(any, string, ...any) error
	QueryRow(string, ...any) *sql.Row
	PrepareNamedContext(context.Context, string) (*sqlx.NamedStmt, error)
	PrepareNamed(string) (*sqlx.NamedStmt, error)
	Preparex(string) (*sqlx.Stmt, error)
	NamedExec(string, any) (sql.Result, error)
	NamedExecContext(context.Context, string, any) (sql.Result, error)
	MustExec(string, ...any) sql.Result
	NamedQuery(string, any) (*sqlx.Rows, error)
}
