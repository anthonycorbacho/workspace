package sql

import (
	"context"
	stdsql "database/sql"
	sqldriver "database/sql/driver"
	"io"
	"io/fs"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"testing/fstest"
	"time"

	"github.com/golang-migrate/migrate/v4/database"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/xo/dburl"
)

var _testMigrations = fstest.MapFS{
	"db/1_create_users.up.sql":   {Data: []byte(`CREATE TABLE users (id TEXT PRIMARY KEY, name TEXT NOT NULL);`)},
	"db/1_create_users.down.sql": {Data: []byte(`DROP TABLE users;`)},
	"db/2_add_email.up.sql":      {Data: []byte(`ALTER TABLE users ADD COLUMN email TEXT;`)},
	"db/2_add_email.down.sql":    {Data: []byte(`ALTER TABLE users DROP COLUMN email;`)},
}

func openTestingDB(t *testing.T) *TestingDB {
	t.Helper()
	if os.Getenv("TESTINGDB_URL") == "" {
		t.Skip("Skipping, no testing database setup via env variable TESTINGDB_URL")
	}
	var tdb TestingDB
	require.NoError(t, tdb.Open(t.Context()))
	t.Cleanup(func() { assert.NoError(t, tdb.Close()) })
	return &tdb
}

// connections returns the number of connections opened to the testing database, ours included.
func connections(t *testing.T, tdb *TestingDB) int {
	t.Helper()
	url, err := dburl.Parse(tdb.DSN)
	require.NoError(t, err)
	var n int
	err = tdb.QueryRowContext(context.Background(),
		`SELECT count(*) FROM pg_stat_activity WHERE datname = $1`, strings.TrimPrefix(url.Path, "/")).Scan(&n)
	require.NoError(t, err)
	return n
}

func columnExists(t *testing.T, tdb *TestingDB, table, column string) bool {
	t.Helper()
	var exists bool
	err := tdb.QueryRowContext(context.Background(), `
		SELECT EXISTS (
			SELECT 1 FROM information_schema.columns WHERE table_name = $1 AND column_name = $2
		)`, table, column).Scan(&exists)
	require.NoError(t, err)
	return exists
}

// migrationVersion returns the migration version of the service.
func migrationVersion(t *testing.T, tdb *TestingDB, service string) int {
	t.Helper()
	var version int
	query := `SELECT version FROM ` + pgx.Identifier{service + "_schema_migrations"}.Sanitize()
	require.NoError(t, tdb.QueryRowContext(context.Background(), query).Scan(&version))
	return version
}

func TestMigrate(t *testing.T) {
	tdb := openTestingDB(t)
	before := connections(t, tdb)

	steps := []struct {
		name        string
		opts        []MigrateOption
		wantVersion int
		wantEmail   bool
	}{
		{name: "all up", wantVersion: 2, wantEmail: true},
		{name: "nothing to apply", wantVersion: 2, wantEmail: true},
		{name: "down to version", opts: []MigrateOption{WithVersion(1)}, wantVersion: 1},
		{name: "same version", opts: []MigrateOption{WithVersion(1)}, wantVersion: 1},
		{name: "up to version", opts: []MigrateOption{WithVersion(2)}, wantVersion: 2, wantEmail: true},
	}
	// the steps depend on each other, they run in order.
	for _, step := range steps {
		t.Run(step.name, func(t *testing.T) {
			require.NoError(t, Migrate(t.Context(), tdb.DSN, "test", _testMigrations, step.opts...))
			assert.Equal(t, step.wantVersion, migrationVersion(t, tdb, "test"))
			assert.True(t, columnExists(t, tdb, "users", "name"))
			assert.Equal(t, step.wantEmail, columnExists(t, tdb, "users", "email"))
		})
	}

	// the migration connections are released.
	assert.Equal(t, before, connections(t, tdb))
}

func TestMigrateWithMigrationsPath(t *testing.T) {
	tdb := openTestingDB(t)

	fs := fstest.MapFS{
		"custom/1_create_items.up.sql":   {Data: []byte(`CREATE TABLE items (id TEXT PRIMARY KEY);`)},
		"custom/1_create_items.down.sql": {Data: []byte(`DROP TABLE items;`)},
	}
	require.NoError(t, Migrate(t.Context(), tdb.DSN, "items", fs, WithMigrationsPath("custom")))
	assert.True(t, columnExists(t, tdb, "items", "id"))
}

func TestMigrateErrors(t *testing.T) {
	tdb := openTestingDB(t)
	canceled, cancel := context.WithCancel(context.Background())
	cancel()

	broken := fstest.MapFS{
		"db/1_broken.up.sql":   {Data: []byte(`CREATE TABLE;`)},
		"db/1_broken.down.sql": {Data: []byte(``)},
	}
	tests := []struct {
		name         string
		ctx          context.Context
		connection   func(t *testing.T) string
		service      string
		fsys         fs.FS
		opts         []MigrateOption
		wantErr      string
		wantCanceled bool
	}{
		{name: "missing service name", service: "", fsys: _testMigrations, wantErr: "service name is required"},
		{name: "missing path", service: "test", fsys: _testMigrations, opts: []MigrateOption{WithMigrationsPath("missing")}},
		{name: "missing version", service: "test", fsys: _testMigrations, opts: []MigrateOption{WithVersion(42)}},
		{
			name:       "invalid connection string",
			connection: func(*testing.T) string { return "not a valid url" },
			service:    "test",
			fsys:       _testMigrations,
		},
		{name: "unreachable database", connection: unreachableDSN, service: "test", fsys: _testMigrations},
		{name: "broken migration", service: "broken", fsys: broken},
		{name: "context canceled", ctx: canceled, service: "test", fsys: _testMigrations, wantCanceled: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, connection := t.Context(), tdb.DSN
			if tt.ctx != nil {
				ctx = tt.ctx
			}
			if tt.connection != nil {
				connection = tt.connection(t)
			}

			err := Migrate(ctx, connection, tt.service, tt.fsys, tt.opts...)
			require.Error(t, err)
			if tt.wantErr != "" {
				assert.ErrorContains(t, err, tt.wantErr)
			}
			if tt.wantCanceled {
				assert.ErrorIs(t, err, context.Canceled)
			}
		})
	}
	// the canceled migration did not run.
	assert.False(t, columnExists(t, tdb, "users", "id"))
}

// cancelFS cancels a context when the file name is opened.
type cancelFS struct {
	fs.FS
	name   string
	cancel context.CancelFunc
}

func (c cancelFS) Open(name string) (fs.File, error) {
	if name == c.name {
		c.cancel()
	}
	return c.FS.Open(name)
}

func TestMigrateStopsWhenContextDone(t *testing.T) {
	tdb := openTestingDB(t)

	// canceled once the second migration is read, before it starts.
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	fsys := cancelFS{FS: _testMigrations, name: "db/2_add_email.up.sql", cancel: cancel}

	err := Migrate(ctx, tdb.DSN, "test", fsys)
	assert.ErrorIs(t, err, context.Canceled)
	assert.False(t, columnExists(t, tdb, "users", "email"), "the migrations stop once the context is done")

	// the database is left in a clean state, the migration can be resumed.
	require.NoError(t, Migrate(t.Context(), tdb.DSN, "test", _testMigrations))
	assert.Equal(t, 2, migrationVersion(t, tdb, "test"))
	assert.True(t, columnExists(t, tdb, "users", "email"))
}

func TestMigrateStopsWhenAdvisoryLockIsContended(t *testing.T) {
	tdb := openTestingDB(t)

	// Hold the exact advisory lock that golang-migrate's pgx/v5 driver acquires for "contended".
	var dbName, schemaName string
	require.NoError(t, tdb.QueryRowContext(t.Context(), `SELECT CURRENT_DATABASE(), CURRENT_SCHEMA()`).Scan(&dbName, &schemaName))
	lockID, err := database.GenerateAdvisoryLockId(dbName, schemaName, "contended_schema_migrations")
	require.NoError(t, err)

	conn, err := tdb.Conn(t.Context())
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })

	_, err = conn.ExecContext(t.Context(), `SELECT pg_advisory_lock($1)`, lockID)
	require.NoError(t, err)
	t.Cleanup(func() {
		_, _ = conn.ExecContext(context.Background(), `SELECT pg_advisory_unlock($1)`, lockID)
	})

	ctx, cancel := context.WithTimeout(t.Context(), 200*time.Millisecond)
	defer cancel()

	start := time.Now()
	err = Migrate(ctx, tdb.DSN, "contended", _testMigrations)
	assert.ErrorIs(t, err, context.DeadlineExceeded)
	assert.Less(t, time.Since(start), 5*time.Second)
}

// fakeMigrationDB simulates the PostgreSQL queries executed by golang-migrate's pgx/v5 driver
// so unit tests can verify context cancellation and lock contention without a live PostgreSQL server.
type fakeMigrationDB struct {
	mu               sync.Mutex
	lockCallCount    int
	unlockCallCount  int
	version          int
	dirty            bool
	hasVersion       bool
	executed         []string
	onLock           func(ctx context.Context, call int) error
	onMigrationQuery func(ctx context.Context, query string) error
}

func (f *fakeMigrationDB) openDB(ctx context.Context, protect *atomic.Bool) *stdsql.DB {
	db := stdsql.OpenDB(&migrationConnector{
		base:    fakeConnector{db: f},
		ctx:     ctx,
		protect: protect,
	})
	db.SetMaxOpenConns(1)
	return db
}

type fakeConnector struct {
	db *fakeMigrationDB
}

func (c fakeConnector) Connect(context.Context) (sqldriver.Conn, error) {
	return &fakeConn{db: c.db}, nil
}

func (c fakeConnector) Driver() sqldriver.Driver { return fakeDriver{} }

type fakeDriver struct{}

func (fakeDriver) Open(string) (sqldriver.Conn, error) { return nil, sqldriver.ErrSkip }

type fakeConn struct {
	db *fakeMigrationDB
}

func (c *fakeConn) Prepare(string) (sqldriver.Stmt, error) { return nil, sqldriver.ErrSkip }
func (c *fakeConn) Close() error                           { return nil }
func (c *fakeConn) Begin() (sqldriver.Tx, error)           { return fakeTx{db: c.db}, nil }
func (c *fakeConn) BeginTx(ctx context.Context, _ sqldriver.TxOptions) (sqldriver.Tx, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return fakeTx{db: c.db}, nil
}
func (c *fakeConn) Ping(ctx context.Context) error { return ctx.Err() }

func (c *fakeConn) QueryContext(ctx context.Context, query string, _ []sqldriver.NamedValue) (sqldriver.Rows, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	switch {
	case strings.Contains(query, "CURRENT_DATABASE()"):
		return &fakeRows{cols: []string{"current_database"}, rows: [][]sqldriver.Value{{"testdb"}}}, nil
	case strings.Contains(query, "CURRENT_SCHEMA()"):
		return &fakeRows{cols: []string{"current_schema"}, rows: [][]sqldriver.Value{{"public"}}}, nil
	case strings.Contains(query, "information_schema.tables"):
		return &fakeRows{cols: []string{"count"}, rows: [][]sqldriver.Value{{int64(1)}}}, nil
	case strings.Contains(query, "SELECT version, dirty"):
		c.db.mu.Lock()
		defer c.db.mu.Unlock()
		if !c.db.hasVersion {
			return &fakeRows{cols: []string{"version", "dirty"}}, nil
		}
		return &fakeRows{
			cols: []string{"version", "dirty"},
			rows: [][]sqldriver.Value{{int64(c.db.version), c.db.dirty}},
		}, nil
	default:
		return nil, sqldriver.ErrSkip
	}
}

func (c *fakeConn) ExecContext(ctx context.Context, query string, args []sqldriver.NamedValue) (sqldriver.Result, error) {
	switch {
	case strings.Contains(query, "pg_advisory_lock"):
		c.db.mu.Lock()
		c.db.lockCallCount++
		call := c.db.lockCallCount
		onLock := c.db.onLock
		c.db.mu.Unlock()
		if onLock != nil {
			if err := onLock(ctx, call); err != nil {
				return nil, err
			}
		}
		return sqldriver.RowsAffected(0), ctx.Err()
	case strings.Contains(query, "pg_advisory_unlock"):
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		c.db.mu.Lock()
		c.db.unlockCallCount++
		c.db.mu.Unlock()
		return sqldriver.RowsAffected(0), nil
	case strings.HasPrefix(query, "TRUNCATE "):
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		c.db.mu.Lock()
		c.db.hasVersion = false
		c.db.mu.Unlock()
		return sqldriver.RowsAffected(0), nil
	case strings.HasPrefix(query, "INSERT INTO "):
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		c.db.mu.Lock()
		c.db.version = int(args[0].Value.(int64))
		c.db.dirty = args[1].Value.(bool)
		c.db.hasVersion = true
		c.db.mu.Unlock()
		return sqldriver.RowsAffected(1), nil
	default:
		if c.db.onMigrationQuery != nil {
			if err := c.db.onMigrationQuery(ctx, query); err != nil {
				return nil, err
			}
		}
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		c.db.mu.Lock()
		c.db.executed = append(c.db.executed, query)
		c.db.mu.Unlock()
		return sqldriver.RowsAffected(0), nil
	}
}

type fakeTx struct{ db *fakeMigrationDB }

func (fakeTx) Commit() error   { return nil }
func (fakeTx) Rollback() error { return nil }

type fakeRows struct {
	cols []string
	rows [][]sqldriver.Value
	idx  int
}

func (r *fakeRows) Columns() []string { return r.cols }
func (r *fakeRows) Close() error      { return nil }
func (r *fakeRows) Next(dest []sqldriver.Value) error {
	if r.idx >= len(r.rows) {
		return io.EOF
	}
	copy(dest, r.rows[r.idx])
	r.idx++
	return nil
}

func TestMigrateHonorsContextDuringDriverInitAndLocking(t *testing.T) {
	tests := []struct {
		name             string
		blockOnLockCall  int
		cancelViaTimeout bool
		wantErr          error
	}{
		{
			name:            "canceled while blocked on ensureVersionTable advisory lock",
			blockOnLockCall: 1,
			wantErr:         context.Canceled,
		},
		{
			name:             "deadline exceeded while blocked on migration advisory lock",
			blockOnLockCall:  2,
			cancelViaTimeout: true,
			wantErr:          context.DeadlineExceeded,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var ctx context.Context
			var cancel context.CancelFunc
			if tt.cancelViaTimeout {
				ctx, cancel = context.WithTimeout(context.Background(), 50*time.Millisecond)
			} else {
				ctx, cancel = context.WithCancel(context.Background())
			}
			defer cancel()

			blocked := make(chan struct{})
			fake := &fakeMigrationDB{
				onLock: func(qctx context.Context, call int) error {
					if call != tt.blockOnLockCall {
						return nil
					}
					close(blocked)
					<-qctx.Done()
					return qctx.Err()
				},
			}

			var protect atomic.Bool
			src, err := iofs.New(_testMigrations, "db")
			require.NoError(t, err)

			done := make(chan error, 1)
			go func() {
				done <- migrateWithSourceAndDB(ctx, src, fake.openDB(ctx, &protect), &protect, "test", &migrateOptions{path: "db"})
			}()

			select {
			case <-blocked:
			case <-time.After(2 * time.Second):
				t.Fatal("timed out waiting for advisory lock call")
			}
			if !tt.cancelViaTimeout {
				cancel()
			}

			select {
			case err := <-done:
				require.ErrorIs(t, err, tt.wantErr)
			case <-time.After(2 * time.Second):
				t.Fatal("Migrate did not return promptly after context cancellation while waiting for advisory lock")
			}

			fake.mu.Lock()
			assert.Empty(t, fake.executed, "no migration statements should have executed")
			fake.mu.Unlock()
		})
	}
}

func TestMigrateDoesNotInterruptInFlightMigrationFile(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var protect atomic.Bool
	fake := &fakeMigrationDB{
		onMigrationQuery: func(qctx context.Context, query string) error {
			// Cancel caller ctx mid-migration; the running SQL statement must receive a
			// non-canceled context so the migration finishes cleanly and leaves dirty=false.
			cancel()
			assert.NoError(t, qctx.Err(), "in-flight migration query must not be canceled")
			return nil
		},
	}

	src, err := iofs.New(_testMigrations, "db")
	require.NoError(t, err)

	err = migrateWithSourceAndDB(ctx, src, fake.openDB(ctx, &protect), &protect, "test", &migrateOptions{path: "db"})
	require.ErrorIs(t, err, context.Canceled)

	fake.mu.Lock()
	defer fake.mu.Unlock()
	assert.Len(t, fake.executed, 1, "first migration finishes, second migration does not start")
	assert.Equal(t, 1, fake.version)
	assert.False(t, fake.dirty, "in-flight migration must commit clean state")
	assert.Equal(t, 2, fake.unlockCallCount, "both ensureVersionTable and Migrate must unlock cleanly")
}
