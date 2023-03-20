package sql

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/anthonycorbacho/workspace/kit/errors"
	"github.com/jmoiron/sqlx"
	"github.com/rs/xid"
	"github.com/xo/dburl"
)

// TestingDB abstracts a database that can be used in tests.
// defaults to connecting to localhost:5432 with username "postgres" and password "postgres".
//
// The environment variable TESTINGDB_URL can be used to control the connection
// settings.
//
// Example usage:
//
//		func TestMe(t *testing.T) {
//			var db sql.TestingDB
//			err := db.Open()
//			if !assert.NoError(t, err) {
//				return
//			}
//			defer db.Close()
//
//	     // connecting to the test database via generated dsn for this test purpose.
//	     database.Open(db.DSN)
//		}
type TestingDB struct {
	*sqlx.DB
	DSN string
}

// Open creates and initializes the testing database.
func (tdb *TestingDB) Open() error {
	// Parse the data source name / pattern
	connection, ok := os.LookupEnv("TESTINGDB_URL")
	if !ok {
		connection = "postgresql://postgres:postgres@localhost:5432/postgres"
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
	url.Path = fmt.Sprintf("%s_%s_%s", url.URL.Path, now.Format("150405"), xid.New().String())
	tdb.DSN = url.URL.String()
	dbName := strings.TrimPrefix(url.Path, "/")

	// connect to the root database
	rootURL := fmt.Sprintf("%s://%s@%s?sslmode=disable", url.Scheme, url.User.String(), url.Host)
	rootdb, err := Open(rootURL)
	if err != nil {
		return err
	}
	defer rootdb.Close()

	// Create the testing database
	_, err = rootdb.Exec(fmt.Sprintf("DROP DATABASE IF EXISTS %s", dbName))
	if err != nil {
		return err
	}

	_, err = rootdb.Exec(fmt.Sprintf("CREATE DATABASE %s", dbName))
	if err != nil {
		return err
	}

	// keep a DB reference to the testingDB
	tdb.DB, err = Open(tdb.DSN)
	if err != nil {
		return err
	}

	return nil
}

// Close cleanup and close the testing database.
func (tdb *TestingDB) Close() error {
	if tdb.DB == nil {
		return nil
	}

	db := tdb.DB
	_ = db.Close()
	tdb.DB = nil

	url, err := dburl.Parse(tdb.DSN)
	if err != nil {
		return err
	}
	dbName := strings.TrimPrefix(url.Path, "/")
	rootURL := fmt.Sprintf("%s://%s@%s?sslmode=disable", url.Scheme, url.User.String(), url.Host)
	db, err = Open(rootURL)
	if err != nil {
		return err
	}
	defer db.Close()

	// Postgres keeps a list of connection open to the DB,
	// we need to close all connections beside ours
	// in order to be able to DROP the testing table.
	// otherwise this will result to an error
	// "Database is being accessed by other users".
	const q = `
	SELECT pg_terminate_backend(pg_stat_activity.pid)
	FROM pg_stat_activity
	WHERE pg_stat_activity.datname = '%s'
	AND pid <> pg_backend_pid();`
	_, err = db.Exec(fmt.Sprintf(q, dbName))
	if err != nil {
		return err
	}

	const d = `DROP DATABASE IF EXISTS %s`
	_, err = db.Exec(fmt.Sprintf(d, dbName))
	return err
}
