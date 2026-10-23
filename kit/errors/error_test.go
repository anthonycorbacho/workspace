package errors

import (
	"fmt"
	"io"
	"io/fs"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNew(t *testing.T) {
	tests := []struct {
		name    string
		message string
	}{
		{name: "message", message: "something failed"},
		{name: "empty message", message: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := New(tt.message)
			require.Error(t, err)
			assert.EqualError(t, err, tt.message)
			// each call returns a distinct error value.
			assert.False(t, Is(err, New(tt.message)))
			assert.True(t, Is(err, err))
		})
	}
}

func TestNewf(t *testing.T) {
	tests := []struct {
		name   string
		format string
		args   []any
		want   string
	}{
		{name: "no args", format: "failed", want: "failed"},
		{name: "with args", format: "failed %s %d", args: []any{"user", 42}, want: "failed user 42"},
		{name: "empty format", format: "", want: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := Newf(tt.format, tt.args...)
			require.Error(t, err)
			assert.EqualError(t, err, tt.want)
		})
	}
}

func TestWrap(t *testing.T) {
	base := New("base")

	tests := []struct {
		name    string
		err     error
		message string
		want    string
	}{
		{name: "nil error returns nil", err: nil, message: "context"},
		{name: "wraps error", err: base, message: "context", want: "context: base"},
		{name: "wraps with empty message", err: base, message: "", want: ": base"},
		{name: "wraps wrapped error", err: Wrap(base, "inner"), message: "outer", want: "outer: inner: base"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Wrap(tt.err, tt.message)
			if tt.err == nil {
				assert.NoError(t, got)
				return
			}
			require.Error(t, got)
			assert.EqualError(t, got, tt.want)
			assert.True(t, Is(got, base))
			assert.Equal(t, tt.err, Unwrap(got))
		})
	}
}

func TestWrapf(t *testing.T) {
	base := New("base")

	tests := []struct {
		name   string
		err    error
		format string
		args   []any
		want   string
	}{
		{name: "nil error returns nil", err: nil, format: "context %d", args: []any{1}},
		{name: "wraps error", err: base, format: "context", want: "context: base"},
		{name: "wraps error with args", err: base, format: "get %s/%d", args: []any{"user", 7}, want: "get user/7: base"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Wrapf(tt.err, tt.format, tt.args...)
			if tt.err == nil {
				assert.NoError(t, got)
				return
			}
			require.Error(t, got)
			assert.EqualError(t, got, tt.want)
			assert.True(t, Is(got, base))
			assert.Equal(t, tt.err, Unwrap(got))
		})
	}
}

func TestUnwrap(t *testing.T) {
	base := New("base")

	tests := []struct {
		name string
		err  error
		want error
	}{
		{name: "nil", err: nil, want: nil},
		{name: "not wrapped", err: base, want: nil},
		{name: "wrapped", err: Wrap(base, "context"), want: base},
		{name: "double wrapped returns one level", err: Wrap(Wrap(base, "inner"), "outer"), want: Wrap(base, "inner")},
		{name: "std wrapped", err: fmt.Errorf("context: %w", base), want: base},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, Unwrap(tt.err))
		})
	}
}

func TestIs(t *testing.T) {
	base := New("base")

	tests := []struct {
		name   string
		err    error
		target error
		want   bool
	}{
		{name: "both nil", err: nil, target: nil, want: true},
		{name: "nil error", err: nil, target: base, want: false},
		{name: "nil target", err: base, target: nil, want: false},
		{name: "same error", err: base, target: base, want: true},
		{name: "same text different error", err: base, target: New("base"), want: false},
		{name: "wrapped", err: Wrap(base, "context"), target: base, want: true},
		{name: "wrapped multiple times", err: Wrapf(Wrap(base, "a"), "b %d", 1), target: base, want: true},
		{name: "std sentinel", err: Wrap(io.EOF, "read"), target: io.EOF, want: true},
		{name: "different sentinel", err: Wrap(io.EOF, "read"), target: io.ErrUnexpectedEOF, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, Is(tt.err, tt.target))
		})
	}
}

func TestAs(t *testing.T) {
	pathErr := &fs.PathError{Op: "open", Path: "/tmp/file", Err: fs.ErrNotExist}

	tests := []struct {
		name string
		err  error
		want bool
	}{
		{name: "nil", err: nil, want: false},
		{name: "not matching", err: New("base"), want: false},
		{name: "direct match", err: pathErr, want: true},
		{name: "wrapped match", err: Wrap(pathErr, "context"), want: true},
		{name: "wrapf match", err: Wrapf(Wrap(pathErr, "a"), "b"), want: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var target *fs.PathError
			got := As(tt.err, &target)
			assert.Equal(t, tt.want, got)
			if tt.want {
				assert.Equal(t, pathErr, target)
				assert.True(t, Is(tt.err, fs.ErrNotExist))
			} else {
				assert.Nil(t, target)
			}
		})
	}
}

func TestAsInvalidTarget(t *testing.T) {
	// As follows the standard library contract and panics on a nil target or a nil pointer target.
	assert.Panics(t, func() { As(New("base"), nil) })
	var target *fs.PathError
	assert.Panics(t, func() { As(New("base"), target) })
}
