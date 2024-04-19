package sql

import (
	"context"
	"fmt"
	"io/fs"
	"time"

	"github.com/golang-migrate/migrate/v4"

	"github.com/anthonycorbacho/workspace/kit/errors"
	"github.com/golang-migrate/migrate/v4/database/pgx"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	"github.com/jmoiron/sqlx"
	"github.com/uptrace/opentelemetry-go-extra/otelsql"
	"github.com/uptrace/opentelemetry-go-extra/otelsqlx"
	"github.com/xo/dburl"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	semconv "go.opentelemetry.io/otel/semconv/v1.18.0"

	// force import pgx (postgres driver)
	_ "github.com/jackc/pgx/v4/stdlib"
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

	// For backward compatibility
	driver := url.Driver
	if driver == "postgres" {
		driver = "pgx"
	}

	otelAttributes := otelsql.WithAttributes(attribute.KeyValue{
		Key:   semconv.DBConnectionStringKey,
		Value: attribute.StringValue(url.Host),
	})

	// Connect to the database using the otel driver wrapper.
	db, err := otelsqlx.Open(driver, connection, otelsql.WithAttributes(semconv.DBSystemPostgreSQL),
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

	db.SetMaxOpenConns(opts.MaxOpenConns)
	db.SetMaxIdleConns(opts.MaxIdleConns)
	db.SetConnMaxLifetime(opts.MaxConnLifeTime)
	db.SetConnMaxIdleTime(opts.MaxConnIdleTime)

	return db, nil
}

// StatusCheck returns nil if it can successfully talk to the database. It
// returns a non-nil error otherwise.
func StatusCheck(ctx context.Context, db *sqlx.DB) error {
	_, span := otel.Tracer("db").Start(ctx, "bd.StatusCheck")
	defer span.End()

	// Run a simple query to determine connectivity. The db has a "Ping" method
	// but it can false-positive when it was previously able to talk to the
	// database but the database has since gone away. Running this query forces a
	// round trip to the database.
	const q = `SELECT true`
	var tmp bool
	return db.QueryRow(q).Scan(&tmp)
}

// Migrate looks at the currently active migration version of the service
// and will migrate all the way up (applying all up migrations).
// Migrate will look at the folder `db` by default (generally assets/db).
func Migrate(db *sqlx.DB, service string, fs fs.FS) error {
	return MigrateWithPath(db, fs, service, "db")
}

// MigrateWithPath looks at the currently active migration version of the service
// and will migrate all the way up (applying all up migrations)
// from the given fs path.
func MigrateWithPath(db *sqlx.DB, fs fs.FS, service string, path string) error {
	m, err := getMigrate(db, fs, service, path)
	if err != nil {
		return err
	}

	// Apply the migration.
	if err := m.Up(); err != nil && err != migrate.ErrNoChange {
		return err
	}

	return nil
}

// MigrateToVersion should be use to apply down or up script to a given version
func MigrateToVersion(db *sqlx.DB, service string, fs fs.FS, version uint) error {
	m, err := getMigrate(db, fs, service, "db")
	if err != nil {
		return err
	}
	return m.Migrate(version)
}

func getMigrate(db *sqlx.DB, fs fs.FS, service string, path string) (*migrate.Migrate, error) {
	if len(service) == 0 {
		return nil, errors.New("service name is required")
	}

	d, err := iofs.New(fs, path)
	if err != nil {
		return nil, err
	}

	driver, err := pgx.WithInstance(db.DB, &pgx.Config{
		MigrationsTable: fmt.Sprintf("%s_schema_migrations", service),
	})
	if err != nil {
		return nil, err
	}
	return migrate.NewWithInstance("iofs", d, db.DriverName(), driver)
}
