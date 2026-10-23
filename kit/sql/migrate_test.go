package sql

import (
	"context"
	"os"
	"strings"
	"testing"
	"testing/fstest"

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
	require.NoError(t, tdb.Open())
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

func TestMigrate(t *testing.T) {
	tdb := openTestingDB(t)
	before := connections(t, tdb)

	require.NoError(t, Migrate(tdb.DSN, "test", _testMigrations))
	assert.True(t, columnExists(t, tdb, "users", "email"))

	// a second migration has nothing to apply.
	require.NoError(t, Migrate(tdb.DSN, "test", _testMigrations))

	var version int
	require.NoError(t, tdb.QueryRowContext(context.Background(), `SELECT version FROM test_schema_migrations`).Scan(&version))
	assert.Equal(t, 2, version)

	require.NoError(t, MigrateToVersion(tdb.DSN, "test", _testMigrations, 1))
	assert.True(t, columnExists(t, tdb, "users", "name"))
	assert.False(t, columnExists(t, tdb, "users", "email"))

	// the migration connections are released.
	assert.Equal(t, before, connections(t, tdb))
}

func TestMigrateWithPath(t *testing.T) {
	tdb := openTestingDB(t)

	fs := fstest.MapFS{
		"custom/1_create_items.up.sql":   {Data: []byte(`CREATE TABLE items (id TEXT PRIMARY KEY);`)},
		"custom/1_create_items.down.sql": {Data: []byte(`DROP TABLE items;`)},
	}
	require.NoError(t, MigrateWithPath(tdb.DSN, "items", fs, "custom"))
	assert.True(t, columnExists(t, tdb, "items", "id"))
}

func TestMigrateErrors(t *testing.T) {
	tdb := openTestingDB(t)

	assert.ErrorContains(t, Migrate(tdb.DSN, "", _testMigrations), "service name is required")
	assert.Error(t, MigrateWithPath(tdb.DSN, "test", _testMigrations, "missing"))
	assert.Error(t, Migrate("not a valid url", "test", _testMigrations))

	broken := fstest.MapFS{
		"db/1_broken.up.sql":   {Data: []byte(`CREATE TABLE;`)},
		"db/1_broken.down.sql": {Data: []byte(``)},
	}
	assert.Error(t, Migrate(tdb.DSN, "broken", broken))
}
