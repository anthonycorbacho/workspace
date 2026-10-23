package sql

import "time"

// options provides a set of configurable options for SQL.
type options struct {
	MaxOpenConns       int
	MaxIdleConns       int
	MaxConnLifeTime    time.Duration
	MaxConnIdleTime    time.Duration
	StatementCacheMode string
}

// Option defines a SQL option.
type Option func(*options)

// WithMaxOpenConns defines the maximum number of open connections to the database.
func WithMaxOpenConns(n int) Option {
	return func(o *options) {
		o.MaxOpenConns = n
	}
}

// WithMaxIdleConns defines the maximum number of connections in the idle connection pool.
func WithMaxIdleConns(n int) Option {
	return func(o *options) {
		o.MaxIdleConns = n
	}
}

// WithMaxConnLifeTime defines the maximum amount of time a connection may be reused.
func WithMaxConnLifeTime(d time.Duration) Option {
	return func(o *options) {
		o.MaxConnLifeTime = d
	}
}

// WithMaxConnIdleTime defines the maximum amount of time a connection may be idle.
func WithMaxConnIdleTime(d time.Duration) Option {
	return func(o *options) {
		o.MaxConnIdleTime = d
	}
}

// WithStatementCacheMode defines how the queries are sent to the database, it sets the pgx
// default_query_exec_mode: cache_statement, cache_describe, describe_exec, exec or simple_protocol.
//
// The pgx v4 statement cache modes are also supported: prepare (cache_statement) and describe (cache_describe).
// Use exec or simple_protocol when connecting through a pooler not supporting prepared statements (eg: pgbouncer).
func WithStatementCacheMode(mode string) Option {
	return func(o *options) {
		o.StatementCacheMode = mode
	}
}
