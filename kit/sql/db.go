package sql

import (
	"context"
	stdsql "database/sql"
	stderrors "errors"
	"time"

	"github.com/anthonycorbacho/workspace/kit/errors"
	"github.com/jmoiron/sqlx"
	"github.com/uptrace/opentelemetry-go-extra/otelsql"
	"github.com/uptrace/opentelemetry-go-extra/otelsqlx"
	"github.com/xo/dburl"
	"go.opentelemetry.io/otel"
	semconv "go.opentelemetry.io/otel/semconv/v1.43.0"

	// force import pgx (postgres driver)
	_ "github.com/jackc/pgx/v5/stdlib"
)

// DB is a pool of database connections instrumented with OpenTelemetry (traces and metrics).
//
// It embeds *sqlx.DB, all the sqlx methods (GetContext, SelectContext, BeginTxx, ...) are available.
// A DB must be created with Open and closed with Close to release its metrics.
type DB struct {
	*sqlx.DB

	// stats holds the registration of the DB stats metrics callbacks, released by Close.
	stats *callbackRecorder
}

// Open opens a database connection pool based on the connection string and checks
// that the database is reachable, it returns an error if the database cannot be pinged
// before ctx is done.
//
// The connection string should follow this format
// postgresql://user:password@host/db[?options]
//
// The database must be closed with Close to release its connections and metrics.
func Open(ctx context.Context, connection string, opts ...Option) (*DB, error) {
	db, err := open(connection, opts...)
	if err != nil {
		return nil, err
	}

	if err := db.PingContext(ctx); err != nil {
		_ = db.Close() //nolint:errcheck // the ping error is more relevant.
		return nil, errors.Wrap(err, "ping db")
	}
	return db, nil
}

// open opens a database connection pool without checking the database is reachable.
func open(connection string, opts ...Option) (*DB, error) {
	url, err := dburl.Parse(connection)
	if err != nil {
		return nil, errors.Wrap(err, "parsing database connection string")
	}

	// Setup SQL options.
	o := &options{
		MaxOpenConns:    10,
		MaxIdleConns:    7,
		MaxConnLifeTime: 30 * time.Minute,
		MaxConnIdleTime: 10 * time.Minute,
	}
	for _, opt := range opts {
		opt(o)
	}

	if len(o.StatementCacheMode) > 0 {
		mode, err := queryExecMode(o.StatementCacheMode)
		if err != nil {
			return nil, err
		}
		query := url.Query()
		query.Set("default_query_exec_mode", mode)
		url.RawQuery = query.Encode()
		connection = url.URL.String()
	}

	// For backward compatibility
	driver := url.Driver
	if driver == "postgres" {
		driver = "pgx"
	}

	// otelsql reports the DB stats (e.g. # sql connection, etc) through a callback it never
	// unregisters, the recorder keeps its registration so Close can release it.
	recorder := newCallbackRecorder()

	// Connect to the database using the otel driver wrapper.
	db, err := otelsqlx.Open(driver, connection,
		otelsql.WithAttributes(semconv.DBSystemNamePostgreSQL, semconv.ServerAddress(url.Hostname())),
		otelsql.WithMeterProvider(recorder),
	)
	if err != nil {
		_ = recorder.unregister() //nolint:errcheck // the open error is more relevant.
		return nil, errors.Wrap(err, "open db")
	}

	db.SetMaxOpenConns(o.MaxOpenConns)
	db.SetMaxIdleConns(o.MaxIdleConns)
	db.SetConnMaxLifetime(o.MaxConnLifeTime)
	db.SetConnMaxIdleTime(o.MaxConnIdleTime)

	return &DB{DB: db, stats: recorder}, nil
}

// queryExecMode returns the pgx default_query_exec_mode of the given statement cache mode.
func queryExecMode(mode string) (string, error) {
	switch mode {
	case "prepare":
		return "cache_statement", nil
	case "describe":
		return "cache_describe", nil
	case "cache_statement", "cache_describe", "describe_exec", "exec", "simple_protocol":
		return mode, nil
	}
	return "", errors.Newf("unknown statement cache mode %q", mode)
}

// Close closes the database connections and releases its metrics.
// Closing an already closed database is a no-op.
func (db *DB) Close() error {
	err := db.DB.Close()
	if db.stats != nil {
		err = stderrors.Join(err, db.stats.unregister())
	}
	return err
}

// StatusCheck returns nil if it can successfully talk to the database. It
// returns a non-nil error otherwise.
func (db *DB) StatusCheck(ctx context.Context) error {
	ctx, span := otel.Tracer("db").Start(ctx, "db.StatusCheck")
	defer span.End()

	// Run a simple query to determine connectivity. The db has a "Ping" method
	// but it can false-positive when it was previously able to talk to the
	// database but the database has since gone away. Running this query forces a
	// round trip to the database.
	const q = `SELECT true`
	var tmp bool
	return db.QueryRowContext(ctx, q).Scan(&tmp)
}

// WithTx runs fn in a transaction: the transaction is committed if fn returns nil and
// rolled back if fn returns an error or panics (the panic is propagated).
//
// The error of fn is returned as is so it can be inspected by the caller.
func (db *DB) WithTx(ctx context.Context, fn func(tx *sqlx.Tx) error) error {
	tx, err := db.BeginTxx(ctx, nil)
	if err != nil {
		return errors.Wrap(err, "begin transaction")
	}
	defer func() {
		if p := recover(); p != nil {
			_ = tx.Rollback() //nolint:errcheck // the panic is more relevant.
			panic(p)
		}
	}()

	if err := fn(tx); err != nil {
		// the transaction is already rolled back when its context is done.
		if rbErr := tx.Rollback(); rbErr != nil && !errors.Is(rbErr, stdsql.ErrTxDone) {
			err = stderrors.Join(err, errors.Wrap(rbErr, "rollback transaction"))
		}
		return err
	}
	return errors.Wrap(tx.Commit(), "commit transaction")
}
