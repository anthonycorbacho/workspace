package sql

import (
	"context"
	"net"
	"os"
	"strings"
	"testing"

	"github.com/anthonycorbacho/workspace/kit/id"
	"github.com/jmoiron/sqlx"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/xo/dburl"
)

// unreachableDSN returns a connection string to a local port nothing is listening on.
func unreachableDSN(t *testing.T) string {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	addr := l.Addr().String()
	require.NoError(t, l.Close())
	return "postgresql://postgres:secret@" + addr + "/postgres?sslmode=disable&connect_timeout=1"
}

func TestOpen(t *testing.T) {
	tests := []struct {
		name         string
		connection   string
		opts         []Option
		wantErr      bool
		wantMaxConns int
	}{
		{
			name:       "invalid connection string",
			connection: "not a valid url",
			wantErr:    true,
		},
		{
			name:       "unknown scheme",
			connection: "unknown://user:pass@localhost/db",
			wantErr:    true,
		},
		{
			name:         "default options",
			connection:   "postgresql://user:pass@localhost/db",
			wantMaxConns: 10,
		},
		{
			name:       "custom options",
			connection: "postgres://user:pass@localhost/db",
			opts: []Option{
				WithMaxOpenConns(3),
				WithMaxIdleConns(2),
				WithMaxConnLifeTime(0),
				WithMaxConnIdleTime(0),
				WithStatementCacheMode("describe"),
			},
			wantMaxConns: 3,
		},
		{
			name:       "unknown statement cache mode",
			connection: "postgres://user:pass@localhost/db",
			opts:       []Option{WithStatementCacheMode("unknown")},
			wantErr:    true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db, err := Open(tt.connection, tt.opts...)
			if tt.wantErr {
				assert.Error(t, err)
				assert.Nil(t, db)
				return
			}
			require.NoError(t, err)
			t.Cleanup(func() { _ = db.Close() })
			assert.Equal(t, "pgx", db.DriverName())
			assert.Equal(t, tt.wantMaxConns, db.Stats().MaxOpenConnections)
		})
	}
}

func TestStatusCheck(t *testing.T) {
	canceled, cancel := context.WithCancel(context.Background())
	cancel()

	tests := []struct {
		name    string
		db      func(t *testing.T) *sqlx.DB
		ctx     context.Context
		wantErr bool
	}{
		{
			name: "database reachable",
			db:   func(t *testing.T) *sqlx.DB { return openTestingDB(t).DB },
			ctx:  context.Background(),
		},
		{
			name: "database unreachable",
			db: func(t *testing.T) *sqlx.DB {
				db, err := Open(unreachableDSN(t))
				require.NoError(t, err)
				t.Cleanup(func() { _ = db.Close() })
				return db
			},
			ctx:     context.Background(),
			wantErr: true,
		},
		{
			name:    "context canceled",
			db:      func(t *testing.T) *sqlx.DB { return openTestingDB(t).DB },
			ctx:     canceled,
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := StatusCheck(tt.ctx, tt.db(t))
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			assert.NoError(t, err)
		})
	}
}

type user struct {
	ID   string `db:"id"`
	Name string `db:"name"`
}

// insertUser inserts a user with any Queryable.
func insertUser(ctx context.Context, q Queryable, u user) error {
	_, err := q.NamedExecContext(ctx, `INSERT INTO users (id, name) VALUES (:id, :name)`, u)
	return err
}

// countUsers counts the users with any Queryable.
func countUsers(t *testing.T, q Queryable) int {
	t.Helper()
	var n int
	require.NoError(t, q.GetContext(context.Background(), &n, `SELECT count(*) FROM users`))
	return n
}

func TestQueryable(t *testing.T) {
	tdb := openTestingDB(t)
	ctx := context.Background()
	tdb.MustExecContext(ctx, `CREATE TABLE users (id TEXT PRIMARY KEY, name TEXT NOT NULL)`)

	tests := []struct {
		name      string
		run       func(t *testing.T) error
		wantErr   bool
		wantCount int
	}{
		{
			name: "insert with db",
			run: func(t *testing.T) error {
				return insertUser(ctx, tdb.DB, user{ID: id.New(), Name: "db"})
			},
			wantCount: 1,
		},
		{
			name: "insert with committed transaction",
			run: func(t *testing.T) error {
				tx, err := tdb.BeginTxx(ctx, nil)
				require.NoError(t, err)
				require.NoError(t, insertUser(ctx, tx, user{ID: id.New(), Name: "commit"}))
				// the transaction sees its own changes.
				assert.Equal(t, 2, countUsers(t, tx))
				return tx.Commit()
			},
			wantCount: 2,
		},
		{
			name: "insert with rolled back transaction",
			run: func(t *testing.T) error {
				tx, err := tdb.BeginTxx(ctx, nil)
				require.NoError(t, err)
				require.NoError(t, insertUser(ctx, tx, user{ID: id.New(), Name: "rollback"}))
				return tx.Rollback()
			},
			wantCount: 2,
		},
		{
			name: "duplicated id",
			run: func(t *testing.T) error {
				u := user{ID: id.New(), Name: "dup"}
				require.NoError(t, insertUser(ctx, tdb.DB, u))
				return insertUser(ctx, tdb.DB, u)
			},
			wantErr:   true,
			wantCount: 3,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.run(t)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
			assert.Equal(t, tt.wantCount, countUsers(t, tdb.DB))
		})
	}

	var users []user
	require.NoError(t, tdb.SelectContext(ctx, &users, `SELECT id, name FROM users WHERE name = $1`, "commit"))
	require.Len(t, users, 1)
	assert.Equal(t, "commit", users[0].Name)
}

// databaseExists reports whether the database of the given DSN exists.
func databaseExists(t *testing.T, tdb *TestingDB, dsn string) bool {
	t.Helper()
	url, err := dburl.Parse(dsn)
	require.NoError(t, err)
	var exists bool
	err = tdb.QueryRowContext(context.Background(),
		`SELECT EXISTS (SELECT 1 FROM pg_database WHERE datname = $1)`, strings.TrimPrefix(url.Path, "/")).Scan(&exists)
	require.NoError(t, err)
	return exists
}

func TestTestingDBLifecycle(t *testing.T) {
	// used to look at the other testing database.
	observer := openTestingDB(t)

	var tdb TestingDB
	require.NoError(t, tdb.Open())
	require.NotNil(t, tdb.DB)
	assert.NotEqual(t, observer.DSN, tdb.DSN, "each testing database is unique")
	assert.Contains(t, tdb.DSN, "sslmode=disable")
	assert.True(t, databaseExists(t, observer, tdb.DSN))
	assert.NoError(t, StatusCheck(context.Background(), tdb.DB))

	// an extra connection to the testing database does not prevent dropping it.
	extra, err := Open(tdb.DSN)
	require.NoError(t, err)
	t.Cleanup(func() { _ = extra.Close() })
	require.NoError(t, StatusCheck(context.Background(), extra))

	require.NoError(t, tdb.Close())
	assert.Nil(t, tdb.DB)
	assert.False(t, databaseExists(t, observer, tdb.DSN))

	// closing again is a no-op.
	assert.NoError(t, tdb.Close())
}

func TestTestingDBOpenErrors(t *testing.T) {
	tests := []struct {
		name       string
		connection func(t *testing.T) string
	}{
		{name: "empty connection string", connection: func(*testing.T) string { return "" }},
		{name: "invalid connection string", connection: func(*testing.T) string { return "not a valid url" }},
		{name: "unreachable database", connection: unreachableDSN},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("TESTINGDB_URL", tt.connection(t))

			var tdb TestingDB
			assert.Error(t, tdb.Open())
			assert.Nil(t, tdb.DB)
			assert.NoError(t, tdb.Close())
		})
	}
}

func TestStatementCacheMode(t *testing.T) {
	if os.Getenv("TESTINGDB_URL") == "" {
		t.Skip("Skipping, no testing database setup via env variable TESTINGDB_URL")
	}
	var tdb TestingDB
	require.NoError(t, tdb.Open())
	t.Cleanup(func() { assert.NoError(t, tdb.Close()) })

	tests := []struct {
		mode         string
		wantPrepared bool
	}{
		{mode: "cache_statement", wantPrepared: true},
		{mode: "prepare", wantPrepared: true},
		{mode: "exec", wantPrepared: false},
		{mode: "simple_protocol", wantPrepared: false},
	}
	for _, tt := range tests {
		t.Run(tt.mode, func(t *testing.T) {
			// a single connection so the prepared statements are checked on the session running the query.
			db, err := Open(tdb.DSN, WithMaxOpenConns(1), WithStatementCacheMode(tt.mode))
			require.NoError(t, err)
			t.Cleanup(func() { _ = db.Close() })

			var v int
			require.NoError(t, db.QueryRowContext(context.Background(), `SELECT $1::int`, 42).Scan(&v))
			assert.Equal(t, 42, v)

			var prepared int
			require.NoError(t, db.QueryRowContext(context.Background(), `SELECT count(*) FROM pg_prepared_statements`).Scan(&prepared))
			assert.Equal(t, tt.wantPrepared, prepared > 0)
		})
	}
}
