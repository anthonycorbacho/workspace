package sql

import (
	"net/url"
	"os"
	"regexp"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTestingdb(t *testing.T) {
	if os.Getenv("TESTINGDB_URL") == "" {
		t.Skip("Skipping, no testing database setup via env variable TESTINGDB_URL")
	}

	// Creating a testing DB
	var tdb TestingDB
	require.NoError(t, tdb.Open(t.Context()))
	defer tdb.Close()

	// Opening a connection to the Test DB
	db, err := Open(t.Context(), tdb.DSN)
	require.NoError(t, err)
	defer db.Close()

	// Make sure we have access to the DB.
	assert.NoError(t, db.StatusCheck(t.Context()))
}

func TestTestingDBDefaultPassword(t *testing.T) {
	compose, err := os.ReadFile("../docker-compose.yaml")
	require.NoError(t, err)
	m := regexp.MustCompile(`POSTGRES_PASSWORD=(\S+)`).FindSubmatch(compose)
	require.NotNil(t, m)

	u, err := url.Parse(defaultTestingDBURL)
	require.NoError(t, err)
	password, _ := u.User.Password()
	assert.Equal(t, string(m[1]), password, "the default connection matches the docker compose database")
}

func TestTestingDBIdentifiers(t *testing.T) {
	// used to look at the other testing databases.
	observer := openTestingDB(t)

	tests := []struct {
		name   string
		dbName string
	}{
		{name: "dash", dbName: "kit-test"},
		{name: "dot and upper case", dbName: "Kit.Test"},
		{name: "space", dbName: "kit test"},
		{name: "double quote", dbName: `kit"test`},
		{name: "single quote", dbName: "kit'test"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			u, err := url.Parse(observer.DSN)
			require.NoError(t, err)
			u.Path = "/" + tt.dbName
			t.Setenv("TESTINGDB_URL", u.String())

			var tdb TestingDB
			require.NoError(t, tdb.Open(t.Context()))
			assert.Contains(t, tdb.DSN, url.PathEscape(tt.dbName))
			assert.True(t, databaseExists(t, observer, tdb.DSN))
			assert.NoError(t, tdb.StatusCheck(t.Context()))

			// an extra connection to the testing database does not prevent dropping it.
			extra, err := Open(t.Context(), tdb.DSN)
			require.NoError(t, err)
			t.Cleanup(func() { _ = extra.Close() })
			require.NoError(t, extra.StatusCheck(t.Context()))

			require.NoError(t, tdb.Close())
			assert.False(t, databaseExists(t, observer, tdb.DSN))
		})
	}
}
