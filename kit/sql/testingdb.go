package sql

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/anthonycorbacho/workspace/kit/errors"
	"github.com/jackc/pgx/v5"
	"github.com/rs/xid"
	"github.com/xo/dburl"
)

// defaultTestingDBURL is the database of kit/docker-compose.yaml.
const defaultTestingDBURL = "postgresql://postgres:secret@localhost:5432/postgres"

// TestingDB is a database dedicated to a test, created by Open and dropped by Close.
// The zero value is ready to use.
//
// It connects by default to localhost:5432 with username "postgres" and password "secret"
// (the database of kit/docker-compose.yaml), the environment variable TESTINGDB_URL
// can be used to control the connection settings.
//
// Example usage:
//
//	func TestMe(t *testing.T) {
//		var tdb sql.TestingDB
//		require.NoError(t, tdb.Open(t.Context()))
//		t.Cleanup(func() { assert.NoError(t, tdb.Close()) })
//
//		// use the testing database connection
//		err := tdb.StatusCheck(t.Context())
//		// or connect to it with the generated DSN
//		db, err := sql.Open(t.Context(), tdb.DSN)
//	}
type TestingDB struct {
	// DB is the connection to the testing database, nil until Open succeeds.
	*DB
	// DSN is the connection string of the testing database.
	DSN string
}

// Open creates the testing database and opens a connection to it.
func (tdb *TestingDB) Open(ctx context.Context) error {
	// Parse the data source name / pattern
	connection, ok := os.LookupEnv("TESTINGDB_URL")
	if !ok {
		connection = defaultTestingDBURL
	}

	if len(connection) == 0 {
		return errors.New("connection string is empty")
	}

	url, err := dburl.Parse(connection)
	if err != nil {
		return err
	}

	query := url.Query()
	query.Set("sslmode", "disable")
	url.RawQuery = query.Encode()

	now := time.Now().UTC()
	url.Path = fmt.Sprintf("%s_%s_%s", url.Path, now.Format("150405"), xid.New().String())
	tdb.DSN = url.URL.String()
	// quoted as the name comes from the connection string and may contain any character (eg: '-').
	dbName := pgx.Identifier{strings.TrimPrefix(url.Path, "/")}.Sanitize()

	// connect to the root database
	rootURL := fmt.Sprintf("%s://%s@%s?sslmode=disable", url.Scheme, url.User.String(), url.Host)
	rootdb, err := Open(ctx, rootURL)
	if err != nil {
		return err
	}
	defer rootdb.Close() //nolint:errcheck // only used to create the testing database.

	// Create the testing database
	_, err = rootdb.ExecContext(ctx, "DROP DATABASE IF EXISTS "+dbName)
	if err != nil {
		return err
	}

	_, err = rootdb.ExecContext(ctx, "CREATE DATABASE "+dbName)
	if err != nil {
		return err
	}

	// keep a DB reference to the testingDB
	db, err := Open(ctx, tdb.DSN)
	if err != nil {
		_, _ = rootdb.ExecContext(ctx, "DROP DATABASE IF EXISTS "+dbName) //nolint:errcheck // the open error is more relevant.
		return err
	}
	tdb.DB = db
	return nil
}

// Close closes the connection to the testing database and drops it.
// Closing a testing database not opened is a no-op.
func (tdb *TestingDB) Close() error {
	if tdb.DB == nil {
		return nil
	}

	_ = tdb.DB.Close() //nolint:errcheck // dropping the testing database is what matters.
	tdb.DB = nil

	// not bound to a test context: Close generally runs in a test cleanup, once the test context is canceled.
	ctx := context.Background()

	url, err := dburl.Parse(tdb.DSN)
	if err != nil {
		return err
	}
	dbName := strings.TrimPrefix(url.Path, "/")
	rootURL := fmt.Sprintf("%s://%s@%s?sslmode=disable", url.Scheme, url.User.String(), url.Host)
	db, err := Open(ctx, rootURL)
	if err != nil {
		return err
	}
	defer db.Close() //nolint:errcheck // the drop error is more relevant.

	// Postgres keeps a list of connection open to the DB,
	// we need to close all connections beside ours
	// in order to be able to DROP the testing database.
	// otherwise this will result to an error
	// "Database is being accessed by other users".
	const q = `
	SELECT pg_terminate_backend(pg_stat_activity.pid)
	FROM pg_stat_activity
	WHERE pg_stat_activity.datname = $1
	AND pid <> pg_backend_pid();`
	_, err = db.ExecContext(ctx, q, dbName)
	if err != nil {
		return err
	}

	_, err = db.ExecContext(ctx, "DROP DATABASE IF EXISTS "+pgx.Identifier{dbName}.Sanitize())
	return err
}
