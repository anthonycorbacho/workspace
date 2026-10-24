package sql

import (
	"context"
	"io/fs"
	"os"
	"strings"
	"testing"
	"testing/fstest"

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
