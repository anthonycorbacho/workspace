package log

import (
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
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

func TestErrorAttrs(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want map[string]any
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
				"errCauses": []map[string]any{
					{"error": "a"},
					{"error": "b"},
				},
			},
		},
		{
			name: "joined stdlib errors",
			err:  errors.Join(errors.New("a"), errors.New("b")),
			want: map[string]any{
				"err": "a\nb",
				"errCauses": []map[string]any{
					{"error": "a"},
					{"error": "b"},
				},
			},
		},
		{
			name: "empty error group",
			err:  groupError{},
			want: map[string]any{"err": "group", "errCauses": []map[string]any{}},
		},
		{name: "nil pointer error", err: (*nilPointerError)(nil), want: map[string]any{"err": "<nil>"}},
		{name: "panicking error", err: panicError{}, want: map[string]any{"err": "PANIC=boom"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := make(map[string]any)
			for _, a := range errorAttrs("err", tt.err) {
				got[a.Key] = a.Value.Any()
			}
			assert.Equal(t, tt.want, got)
		})
	}
}
