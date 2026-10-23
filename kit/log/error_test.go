package log

import (
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"go.uber.org/zap/zapcore"
)

// verboseError is an error with a different verbose (%+v) representation.
type verboseError struct{ msg string }

func (e verboseError) Error() string { return e.msg }

func (e verboseError) Format(s fmt.State, verb rune) {
	if verb == 'v' && s.Flag('+') {
		_, _ = fmt.Fprintf(s, "%s\nstack trace", e.msg)
		return
	}
	_, _ = fmt.Fprint(s, e.msg)
}

// plainFormatterError is a fmt.Formatter error with the same verbose representation.
type plainFormatterError struct{ msg string }

func (e plainFormatterError) Error() string { return e.msg }

func (e plainFormatterError) Format(s fmt.State, _ rune) { _, _ = fmt.Fprint(s, e.msg) }

// groupError is an error made of several errors.
type groupError struct{ errs []error }

func (e groupError) Error() string { return "group" }

func (e groupError) Errors() []error { return e.errs }

type nilPointerError struct{ msg string }

func (e *nilPointerError) Error() string { return e.msg }

type panicError struct{}

func (panicError) Error() string { panic("boom") }

func TestEncodeError(t *testing.T) {
	tests := []struct {
		name    string
		err     error
		want    map[string]any
		wantErr bool
	}{
		{name: "basic error", err: errors.New("boom"), want: map[string]any{"err": "boom"}},
		{
			name: "verbose error",
			err:  verboseError{msg: "boom"},
			want: map[string]any{"err": "boom", "errVerbose": "boom\nstack trace"},
		},
		{
			name: "formatter error without extra verbose",
			err:  plainFormatterError{msg: "boom"},
			want: map[string]any{"err": "boom"},
		},
		{
			name: "error group",
			err:  groupError{errs: []error{errors.New("a"), nil, errors.New("b")}},
			want: map[string]any{
				"err": "group",
				"errCauses": []any{
					map[string]any{"error": "a"},
					map[string]any{"error": "b"},
				},
			},
		},
		{
			name: "empty error group",
			err:  groupError{},
			want: map[string]any{"err": "group", "errCauses": []any{}},
		},
		{name: "nil pointer error", err: (*nilPointerError)(nil), want: map[string]any{"err": "<nil>"}},
		{name: "panicking error", err: panicError{}, want: map[string]any{}, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			enc := zapcore.NewMapObjectEncoder()
			err := encodeError("err", tt.err, enc)
			if tt.wantErr {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), "PANIC=")
			} else {
				assert.NoError(t, err)
			}
			assert.Equal(t, tt.want, enc.Fields)
		})
	}
}

func TestErrArrayElem(t *testing.T) {
	el := newErrArrayElem(errors.New("boom"))
	enc := zapcore.NewMapObjectEncoder()
	assert.NoError(t, enc.AddArray("errs", el))
	assert.Equal(t, map[string]any{"errs": []any{map[string]any{"error": "boom"}}}, enc.Fields)

	el.Free()
	assert.Nil(t, el.err)
}
