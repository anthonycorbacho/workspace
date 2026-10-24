// Package sql manages PostgreSQL connections instrumented with OpenTelemetry,
// the service migrations and the testing databases.
//
// Open opens a connection pool and checks the database is reachable, the returned DB embeds
// *sqlx.DB and must be closed with Close to release its connections and metrics:
//
//	db, err := sql.Open(ctx, "postgresql://user:password@host/db[?options]")
//	if err != nil {
//		return err
//	}
//	defer db.Close()
//
// Migrate applies the migrations of a service embedded in the binary (the db directory by default):
//
//	//go:embed db
//	var migrations embed.FS
//
//	err := sql.Migrate(ctx, connection, "users", migrations)
//
// Queryable is implemented by DB and *sqlx.Tx, a storage can run its queries
// either on the database or in a transaction (see DB.WithTx):
//
//	func insertUser(ctx context.Context, q sql.Queryable, u User) error {
//		_, err := q.NamedExecContext(ctx, `INSERT INTO users (id, name) VALUES (:id, :name)`, u)
//		return err
//	}
//
//	err := db.WithTx(ctx, func(tx *sqlx.Tx) error {
//		return insertUser(ctx, tx, u)
//	})
//
// TestingDB creates a database dedicated to a test.
package sql
