package sql

import (
	"context"
	stdsql "database/sql"
	sqldriver "database/sql/driver"
	stderrors "errors"
	"fmt"
	"io"
	"io/fs"
	"math"
	"sync/atomic"
	"time"

	"github.com/anthonycorbacho/workspace/kit/errors"
	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database"
	pgx "github.com/golang-migrate/migrate/v4/database/pgx/v5"
	"github.com/golang-migrate/migrate/v4/source"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	pgxstdlib "github.com/jackc/pgx/v5/stdlib"
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
// When ctx is done, Migrate stops waiting for driver initialization or the migration
// advisory lock, and stops before starting the next migration file (a running migration
// is not interrupted, leaving the database in a clean state) and returns the ctx error:
// the migrations applied so far are kept, the migration can be resumed by calling Migrate again.
func Migrate(ctx context.Context, connection, service string, fsys fs.FS, opts ...MigrateOption) error {
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

	var protect atomic.Bool
	db, err := openMigrationDB(ctx, connection, &protect)
	if err != nil {
		_ = d.Close() //nolint:errcheck // the open error is more relevant.
		return err
	}
	return migrateWithSourceAndDB(ctx, d, db, &protect, service, o)
}

// migrateWithSourceAndDB runs the migration flow against an opened iofs source and dedicated *sql.DB
// whose connector honors protect. Extracted so unit tests can exercise driver initialization,
// advisory-lock contention, and in-flight migration protection with a fake SQL driver.
func migrateWithSourceAndDB(ctx context.Context, d source.Driver, db *stdsql.DB, protect *atomic.Bool, service string, o *migrateOptions) (err error) {
	// A dedicated *sql.DB is required because the migration driver holds a connection
	// that is only released by closing the whole *sql.DB.
	driver, err := pgx.WithInstance(db, &pgx.Config{
		MigrationsTable: fmt.Sprintf("%s_schema_migrations", service),
	})
	if err != nil {
		_ = d.Close()  //nolint:errcheck // the driver error is more relevant.
		_ = db.Close() //nolint:errcheck // the driver error is more relevant.
		return errors.Wrap(migrationCtxErr(ctx, err), "creating migration driver")
	}

	m, err := migrate.NewWithInstance("iofs", d, pgxDriverName, &contextDriver{
		Driver:  driver,
		ctx:     ctx,
		protect: protect,
	})
	if err != nil {
		_ = d.Close()      //nolint:errcheck // the migrate error is more relevant.
		_ = driver.Close() //nolint:errcheck // the migrate error is more relevant.
		return errors.Wrap(err, "creating migration")
	}
	if deadline, ok := ctx.Deadline(); ok {
		if remaining := time.Until(deadline); remaining > m.LockTimeout {
			m.LockTimeout = remaining + time.Second
		}
	} else {
		// Let ctx govern how long lock acquisition waits rather than failing prematurely
		// after golang-migrate's 15s default while another pod is running a slow migration.
		m.LockTimeout = math.MaxInt64
	}
	defer func() {
		// Unlock and Close run after migrations finish or fail; protect them from a
		// canceled ctx so cleanup still releases the advisory lock cleanly.
		protect.Store(true)
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
		return errors.Wrap(migrationCtxErr(ctx, err), "running migration")
	}
	return nil
}

// contextDriver is a migration database driver stopping the migrations once its context is done.
//
// golang-migrate does not support contexts and its GracefulStop channel is racy:
//   - while outside a running migration file (driver setup, Lock, Version), the underlying SQL
//     connection injects ctx so blocked queries like pg_advisory_lock cancel promptly.
//   - once a migration file begins (SetVersion(v, true)) until it commits cleanly
//     (SetVersion(v, false)), protect is enabled so an in-flight migration file is never aborted
//     mid-statement, leaving the database in a clean state.
type contextDriver struct {
	database.Driver
	ctx     context.Context
	protect *atomic.Bool
}

// Lock acquires the migration advisory lock, honoring ctx cancellation before and during the wait.
func (d *contextDriver) Lock() error {
	if err := d.ctx.Err(); err != nil {
		return err
	}
	return migrationCtxErr(d.ctx, d.Driver.Lock())
}

// Unlock releases the migration advisory lock even after ctx has been canceled.
func (d *contextDriver) Unlock() error {
	d.protect.Store(true)
	defer d.protect.Store(false)
	return d.Driver.Unlock()
}

// SetVersion sets the migration version, failing before starting a new migration (dirty == true)
// once ctx is done, and keeping ctx protection active until the clean version is written.
func (d *contextDriver) SetVersion(version int, dirty bool) error {
	if dirty {
		if err := d.ctx.Err(); err != nil {
			return err
		}
		// Protect the dirty-version write, the following Run, and the clean-version write.
		d.protect.Store(true)
		if err := d.Driver.SetVersion(version, true); err != nil {
			d.protect.Store(false)
			return err
		}
		return nil
	}
	defer d.protect.Store(false)
	return d.Driver.SetVersion(version, false)
}

// Run executes a migration file with cancellation protection enabled so an already-started
// migration finishes cleanly rather than leaving the schema dirty.
func (d *contextDriver) Run(migration io.Reader) error {
	d.protect.Store(true)
	return d.Driver.Run(migration)
}

// migrationCtxErr returns ctx.Err() when err failed because ctx was canceled or its deadline
// expired, because golang-migrate wraps driver errors in database.Error without implementing Unwrap.
func migrationCtxErr(ctx context.Context, err error) error {
	if err != nil && ctx.Err() != nil {
		return ctx.Err()
	}
	return err
}

// openMigrationDB opens a single connection database without instrumentation,
// it is only used for the duration of a migration.
func openMigrationDB(ctx context.Context, connection string, protect *atomic.Bool) (*stdsql.DB, error) {
	url, err := dburl.Parse(connection)
	if err != nil {
		return nil, errors.Wrap(err, "parsing database connection string")
	}

	driverName := url.Driver
	if driverName == "postgres" {
		driverName = pgxDriverName
	}

	connector, err := openMigrationConnector(driverName, connection, ctx, protect)
	if err != nil {
		return nil, err
	}

	db := stdsql.OpenDB(connector)
	db.SetMaxOpenConns(1)

	if err := db.PingContext(ctx); err != nil {
		_ = db.Close() //nolint:errcheck // the ping error is more relevant.
		return nil, errors.Wrap(err, "ping db")
	}
	return db, nil
}

func openMigrationConnector(driverName, connection string, ctx context.Context, protect *atomic.Bool) (sqldriver.Connector, error) {
	var base sqldriver.Connector
	switch driverName {
	case pgxDriverName, "pgx/v5":
		connector, err := pgxstdlib.GetDefaultDriver().(sqldriver.DriverContext).OpenConnector(connection)
		if err != nil {
			return nil, errors.Wrap(err, "open db")
		}
		base = connector
	default:
		db, err := stdsql.Open(driverName, connection)
		if err != nil {
			return nil, errors.Wrap(err, "open db")
		}
		base = openDriverConnector{driver: db.Driver(), dsn: connection}
		_ = db.Close() //nolint:errcheck // temporary handle used only to resolve the registered driver.
	}
	return &migrationConnector{base: base, ctx: ctx, protect: protect}, nil
}

type openDriverConnector struct {
	driver sqldriver.Driver
	dsn    string
}

func (c openDriverConnector) Connect(ctx context.Context) (sqldriver.Conn, error) {
	if dc, ok := c.driver.(sqldriver.DriverContext); ok {
		conn, err := dc.OpenConnector(c.dsn)
		if err != nil {
			return nil, err
		}
		return conn.Connect(ctx)
	}
	return c.driver.Open(c.dsn)
}

func (c openDriverConnector) Driver() sqldriver.Driver {
	return c.driver
}

// migrationConnector wraps a database/sql/driver.Connector so queries issued with
// context.Background() by golang-migrate inherit ctx while protect is false.
type migrationConnector struct {
	base    sqldriver.Connector
	ctx     context.Context
	protect *atomic.Bool
}

func (c *migrationConnector) Connect(ctx context.Context) (sqldriver.Conn, error) {
	conn, err := c.base.Connect(c.effectiveCtx(ctx))
	if err != nil {
		return nil, err
	}
	return &migrationConn{Conn: conn, ctx: c.ctx, protect: c.protect}, nil
}

func (c *migrationConnector) Driver() sqldriver.Driver {
	return c.base.Driver()
}

func (c *migrationConnector) effectiveCtx(callCtx context.Context) context.Context {
	if c.protect.Load() {
		return callCtx
	}
	if _, hasDeadline := callCtx.Deadline(); hasDeadline || callCtx.Done() != nil {
		return callCtx
	}
	return c.ctx
}

type migrationConn struct {
	sqldriver.Conn
	ctx     context.Context
	protect *atomic.Bool
}

func (c *migrationConn) effectiveCtx(callCtx context.Context) context.Context {
	if c.protect.Load() {
		return callCtx
	}
	if _, hasDeadline := callCtx.Deadline(); hasDeadline || callCtx.Done() != nil {
		return callCtx
	}
	return c.ctx
}

func (c *migrationConn) Ping(ctx context.Context) error {
	if pinger, ok := c.Conn.(sqldriver.Pinger); ok {
		return pinger.Ping(c.effectiveCtx(ctx))
	}
	return nil
}

func (c *migrationConn) ExecContext(ctx context.Context, query string, args []sqldriver.NamedValue) (sqldriver.Result, error) {
	execer, ok := c.Conn.(sqldriver.ExecerContext)
	if !ok {
		return nil, sqldriver.ErrSkip
	}
	return execer.ExecContext(c.effectiveCtx(ctx), query, args)
}

func (c *migrationConn) QueryContext(ctx context.Context, query string, args []sqldriver.NamedValue) (sqldriver.Rows, error) {
	queryer, ok := c.Conn.(sqldriver.QueryerContext)
	if !ok {
		return nil, sqldriver.ErrSkip
	}
	return queryer.QueryContext(c.effectiveCtx(ctx), query, args)
}

func (c *migrationConn) PrepareContext(ctx context.Context, query string) (sqldriver.Stmt, error) {
	if preparer, ok := c.Conn.(sqldriver.ConnPrepareContext); ok {
		return preparer.PrepareContext(c.effectiveCtx(ctx), query)
	}
	return c.Prepare(query)
}

func (c *migrationConn) BeginTx(ctx context.Context, opts sqldriver.TxOptions) (sqldriver.Tx, error) {
	if beginner, ok := c.Conn.(sqldriver.ConnBeginTx); ok {
		return beginner.BeginTx(c.effectiveCtx(ctx), opts)
	}
	return c.Begin() //nolint:staticcheck // fallback for drivers not implementing ConnBeginTx.
}

func (c *migrationConn) ResetSession(ctx context.Context) error {
	if resetter, ok := c.Conn.(sqldriver.SessionResetter); ok {
		return resetter.ResetSession(c.effectiveCtx(ctx))
	}
	return nil
}

func (c *migrationConn) CheckNamedValue(nv *sqldriver.NamedValue) error {
	if checker, ok := c.Conn.(sqldriver.NamedValueChecker); ok {
		return checker.CheckNamedValue(nv)
	}
	return sqldriver.ErrSkip
}
