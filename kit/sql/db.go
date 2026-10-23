package sql

import (
	"context"
	stdsql "database/sql"
	stderrors "errors"
	"fmt"
	"io/fs"
	"time"

	"github.com/anthonycorbacho/workspace/kit/errors"
	"github.com/golang-migrate/migrate/v4"
	pgx "github.com/golang-migrate/migrate/v4/database/pgx/v5"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	"github.com/jmoiron/sqlx"
	"github.com/uptrace/opentelemetry-go-extra/otelsql"
	"github.com/uptrace/opentelemetry-go-extra/otelsqlx"
	"github.com/xo/dburl"
	"go.opentelemetry.io/otel"
	semconv "go.opentelemetry.io/otel/semconv/v1.43.0"

	// force import pgx (postgres driver)
	_ "github.com/jackc/pgx/v5/stdlib"
)

// Open knows how to open a database connection based on connection string.
//
// connection string should follow this format
// postgresql://user:password@host/db[?options]
func Open(connection string, ops ...Option) (*sqlx.DB, error) {
	url, err := dburl.Parse(connection)
	if err != nil {
		err = errors.Wrap(err, "parsing database connection string")
		return nil, err
	}

	// Setup SQL options.
	opts := &options{
		MaxOpenConns:    10,
		MaxIdleConns:    7,
		MaxConnLifeTime: 30 * time.Minute,
		MaxConnIdleTime: 10 * time.Minute,
	}
	for _, o := range ops {
		o(opts)
	}

	if len(opts.StatementCacheMode) > 0 {
		mode, err := queryExecMode(opts.StatementCacheMode)
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

	otelAttributes := otelsql.WithAttributes(semconv.ServerAddress(url.Hostname()))

	// Connect to the database using the otel driver wrapper.
	db, err := otelsqlx.Open(driver, connection, otelsql.WithAttributes(semconv.DBSystemNamePostgreSQL),
		otelAttributes,
	)
	if err != nil {
		return nil, errors.Wrap(err, "open db")
	}

	// By default, otelsqlx do not record DB stats.
	// In order to get DB stats (e.g. # sql connection, etc) we need to call ReportDBStatsMetrics
	// and pass sql.DB pointer.
	// this is only available from otelsql pkg.
	otelsql.ReportDBStatsMetrics(db.DB, otelAttributes)

	db.SetMaxOpenConns(opts.MaxOpenConns)
	db.SetMaxIdleConns(opts.MaxIdleConns)
	db.SetConnMaxLifetime(opts.MaxConnLifeTime)
	db.SetConnMaxIdleTime(opts.MaxConnIdleTime)

	return db, nil
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

// StatusCheck returns nil if it can successfully talk to the database. It
// returns a non-nil error otherwise.
func StatusCheck(ctx context.Context, db *sqlx.DB) error {
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

// Migrate looks at the currently active migration version of the service
// and will migrate all the way up (applying all up migrations).
// Migrate will look at the folder `db` by default (generally assets/db).
//
// The migration runs on its own connection opened from the given connection string
// (see Open for the format) and closed once done.
func Migrate(connection string, service string, fs fs.FS) error {
	return MigrateWithPath(connection, service, fs, "db")
}

// MigrateWithPath looks at the currently active migration version of the service
// and will migrate all the way up (applying all up migrations)
// from the given fs path.
func MigrateWithPath(connection string, service string, fs fs.FS, path string) error {
	return runMigration(connection, service, fs, path, func(m *migrate.Migrate) error {
		return m.Up()
	})
}

// MigrateToVersion should be use to apply down or up script to a given version.
func MigrateToVersion(connection string, service string, fs fs.FS, version uint) error {
	return runMigration(connection, service, fs, "db", func(m *migrate.Migrate) error {
		return m.Migrate(version)
	})
}

// runMigration runs the given migration on a dedicated database connection.
//
// A dedicated connection is required because the migration driver holds a connection
// that is only released by closing the whole *sql.DB.
func runMigration(connection string, service string, fs fs.FS, path string, run func(m *migrate.Migrate) error) (err error) {
	if len(service) == 0 {
		return errors.New("service name is required")
	}

	d, err := iofs.New(fs, path)
	if err != nil {
		return errors.Wrap(err, "reading migrations")
	}

	db, err := openMigrationDB(connection)
	if err != nil {
		_ = d.Close() //nolint: the open error is more relevant
		return err
	}

	driver, err := pgx.WithInstance(db, &pgx.Config{
		MigrationsTable: fmt.Sprintf("%s_schema_migrations", service),
	})
	if err != nil {
		_ = d.Close()  //nolint: the driver error is more relevant
		_ = db.Close() //nolint: the driver error is more relevant
		return errors.Wrap(err, "creating migration driver")
	}

	m, err := migrate.NewWithInstance("iofs", d, "pgx", driver)
	if err != nil {
		_ = d.Close()      //nolint: the migrate error is more relevant
		_ = driver.Close() //nolint: the migrate error is more relevant
		return errors.Wrap(err, "creating migration")
	}
	defer func() {
		// closes the source, the driver connection and the dedicated *sql.DB.
		srcErr, dbErr := m.Close()
		if err == nil {
			err = stderrors.Join(srcErr, dbErr)
		}
	}()

	if err := run(m); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return errors.Wrap(err, "running migration")
	}
	return nil
}

// openMigrationDB opens a single connection database without instrumentation,
// it is only used for the duration of a migration.
func openMigrationDB(connection string) (*stdsql.DB, error) {
	url, err := dburl.Parse(connection)
	if err != nil {
		return nil, errors.Wrap(err, "parsing database connection string")
	}

	driver := url.Driver
	if driver == "postgres" {
		driver = "pgx"
	}

	db, err := stdsql.Open(driver, connection)
	if err != nil {
		return nil, errors.Wrap(err, "open db")
	}
	db.SetMaxOpenConns(1)
	return db, nil
}
