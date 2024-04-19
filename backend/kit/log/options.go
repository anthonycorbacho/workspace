package log

// Option provide a set of optional configuration
// that can be provided when creating a logger.
type Option struct {
	Level Level
}

// WithLevel set up the logger log level.
func WithLevel(level Level) func(*Option) {
	return func(o *Option) {
		o.Level = level
	}
}
