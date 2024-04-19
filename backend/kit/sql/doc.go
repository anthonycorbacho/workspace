// Package sql provides function for managing connection to various database.
// You can open a database connection by calling Open function.
//
//	connection := "postgresql://user:password@host/db[?options]"
//	db, _ := sql.Open(connection)
package sql
