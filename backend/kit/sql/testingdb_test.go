package sql

import (
	"context"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestTestingdb(t *testing.T) {
	if os.Getenv("TESTINGDB_URL") == "" {
		t.Skip("Skipping, no testing database setup via env variable TESTINGDB_URL")
	}

	// Creating a testing DB
	var tdb TestingDB
	err := tdb.Open()
	assert.NoError(t, err)
	defer tdb.Close()

	// Opening a connection to the Test DB
	db, err := Open(tdb.DSN)
	assert.NoError(t, err)
	defer db.Close()

	// Make sure we have access to the DB.
	err = StatusCheck(context.Background(), db)
	assert.NoError(t, err)
}
