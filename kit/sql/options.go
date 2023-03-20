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

// WithStatementCacheMode defines the maximum amount of time a connection may be idle.
func WithStatementCacheMode(mode string) Option {
	return func(o *options) {
		o.StatementCacheMode = mode
	}
}
