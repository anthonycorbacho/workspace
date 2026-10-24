package log

import (
	"io"
	"os"
)

// options are the configurable options of a Logger.
type options struct {
	level  Level
	output io.Writer
}

// Option configures a Logger created by New.
type Option func(*options)

// WithLevel sets the minimum level of the logged messages, it overrides FOUNDATION_LOG_LEVEL.
func WithLevel(level Level) Option {
	return func(o *options) {
		o.level = level
	}
}

// WithOutput sets where the logs are written, standard error by default. A nil writer is ignored.
func WithOutput(w io.Writer) Option {
	return func(o *options) {
		if w != nil {
			o.output = w
		}
	}
}

func defaultOptions(level Level) *options {
	return &options{level: level, output: os.Stderr}
}
