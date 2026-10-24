package sql

import (
	"context"
	stdsql "database/sql"
	stderrors "errors"
	"fmt"
	"io/fs"

	"github.com/anthonycorbacho/workspace/kit/errors"
	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database"
	pgx "github.com/golang-migrate/migrate/v4/database/pgx/v5"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	"github.com/xo/dburl"
)

// migrateOptions provides a set of configurable options for the migrations.
type migrateOptions struct {
	path    string
	version *uint
}

// MigrateOption defines a migration option.
type MigrateOption func(*migrateOptions)

// WithMigrationsPath defines the directory of the migration files in the fs.FS, db by default.
func WithMigrationsPath(path string) MigrateOption {
	return func(o *migrateOptions) {
		o.path = path
	}
}

// WithVersion migrates up or down to the given version instead of applying all the up migrations.
func WithVersion(version uint) MigrateOption {
	return func(o *migrateOptions) {
		o.version = &version
	}
}

// Migrate applies the migrations of the service read from fsys: by default it looks at the
// currently active migration version of the service and migrates all the way up (applying all
// up migrations) with the files of the db directory (generally assets/db).
// Use WithVersion to migrate up or down to a given version and WithMigrationsPath to read
// the migrations from another directory.
//
// The service migration version is stored in the <service>_schema_migrations table.
//
// The migration runs on its own connection opened from the given connection string
// (see Open for the format) and closed once done.
//
// When ctx is done, Migrate stops before starting the next migration file (a running migration
// is not interrupted, leaving the database in a clean state) and returns the ctx error:
// the migrations applied so far are kept, the migration can be resumed by calling Migrate again.
func Migrate(ctx context.Context, connection, service string, fsys fs.FS, opts ...MigrateOption) (err error) {
	if len(service) == 0 {
		return errors.New("service name is required")
	}

	o := &migrateOptions{path: "db"}
	for _, opt := range opts {
		opt(o)
	}

	d, err := iofs.New(fsys, o.path)
	if err != nil {
		return errors.Wrap(err, "reading migrations")
	}

	db, err := openMigrationDB(ctx, connection)
	if err != nil {
		_ = d.Close() //nolint:errcheck // the open error is more relevant.
		return err
	}

	// A dedicated *sql.DB is required because the migration driver holds a connection
	// that is only released by closing the whole *sql.DB.
	driver, err := pgx.WithInstance(db, &pgx.Config{
		MigrationsTable: fmt.Sprintf("%s_schema_migrations", service),
	})
	if err != nil {
		_ = d.Close()  //nolint:errcheck // the driver error is more relevant.
		_ = db.Close() //nolint:errcheck // the driver error is more relevant.
		return errors.Wrap(err, "creating migration driver")
	}

	m, err := migrate.NewWithInstance("iofs", d, "pgx", contextDriver{Driver: driver, ctx: ctx})
	if err != nil {
		_ = d.Close()      //nolint:errcheck // the migrate error is more relevant.
		_ = driver.Close() //nolint:errcheck // the migrate error is more relevant.
		return errors.Wrap(err, "creating migration")
	}
	defer func() {
		// closes the source, the driver connection and the dedicated *sql.DB.
		srcErr, dbErr := m.Close()
		if err == nil {
			err = stderrors.Join(srcErr, dbErr)
		}
	}()

	if o.version != nil {
		err = m.Migrate(*o.version)
	} else {
		err = m.Up()
	}
	if err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return errors.Wrap(err, "running migration")
	}
	return nil
}

// contextDriver is a migration database driver stopping the migrations once its context is done.
//
// golang-migrate does not support contexts and its GracefulStop channel is racy,
// the context is checked when a migration starts: each migration first marks its version dirty.
type contextDriver struct {
	database.Driver
	ctx context.Context
}

// SetVersion sets the migration version, it fails without changing the version when a migration
// starts (dirty version) once the context is done.
func (d contextDriver) SetVersion(version int, dirty bool) error {
	if dirty {
		if err := d.ctx.Err(); err != nil {
			return err
		}
	}
	return d.Driver.SetVersion(version, dirty)
}

// openMigrationDB opens a single connection database without instrumentation,
// it is only used for the duration of a migration.
func openMigrationDB(ctx context.Context, connection string) (*stdsql.DB, error) {
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

	if err := db.PingContext(ctx); err != nil {
		_ = db.Close() //nolint:errcheck // the ping error is more relevant.
		return nil, errors.Wrap(err, "ping db")
	}
	return db, nil
}
