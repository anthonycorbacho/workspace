package log

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type stringer string

func (s stringer) String() string { return string(s) }

type panicStringer struct{}

func (*panicStringer) String() string { panic("boom") }

func TestFields(t *testing.T) {
	now := time.Date(2026, 11, 23, 12, 0, 0, 0, time.UTC)

	tests := []struct {
		name  string
		field Field
		want  map[string]any
	}{
		{name: "string", field: String("k", "v"), want: map[string]any{"k": "v"}},
		{name: "strings", field: Strings("k", []string{"a", "b"}), want: map[string]any{"k": []any{"a", "b"}}},
		{name: "byte string", field: ByteString("k", []byte("v")), want: map[string]any{"k": "v"}},
		{name: "stringer", field: Stringer("k", stringer("v")), want: map[string]any{"k": "v"}},
		{name: "nil stringer interface", field: Stringer("k", nil), want: map[string]any{"k": "<nil>"}},
		{name: "typed nil stringer", field: Stringer("k", (*panicStringer)(nil)), want: map[string]any{"k": "<nil>"}},
		{name: "panicking stringer", field: Stringer("k", &panicStringer{}), want: map[string]any{"k": "PANIC=boom"}},
		{name: "bool true", field: Bool("k", true), want: map[string]any{"k": true}},
		{name: "bool false", field: Bool("k", false), want: map[string]any{"k": false}},
		{name: "int", field: Int("k", -1), want: map[string]any{"k": float64(-1)}},
		{name: "int32", field: Int32("k", -2), want: map[string]any{"k": float64(-2)}},
		{name: "int64", field: Int64("k", -3), want: map[string]any{"k": float64(-3)}},
		{name: "uint", field: Uint("k", 1), want: map[string]any{"k": float64(1)}},
		{name: "uint32", field: Uint32("k", 2), want: map[string]any{"k": float64(2)}},
		{name: "uint64", field: Uint64("k", 3), want: map[string]any{"k": float64(3)}},
		{name: "float32", field: Float32("k", 1.5), want: map[string]any{"k": float64(1.5)}},
		{name: "float64", field: Float64("k", 2.5), want: map[string]any{"k": float64(2.5)}},
		{name: "error", field: Error(errors.New("boom")), want: map[string]any{"error": "boom"}},
		{name: "nil error is skipped", field: Error(nil), want: map[string]any{}},
		{name: "duration", field: Duration("k", time.Second), want: map[string]any{"k": float64(time.Second)}},
		{name: "time", field: Time("k", now), want: map[string]any{"k": now.Format(time.RFC3339Nano)}},
		{name: "any string", field: Any("k", "v"), want: map[string]any{"k": "v"}},
		{name: "any struct", field: Any("k", struct{ A int }{A: 1}), want: map[string]any{"k": map[string]any{"A": float64(1)}}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			l, out := newTestLogger(InfoLevel)
			l.Info(context.Background(), "msg", tt.field)

			entries := out.entries(t)
			require.Len(t, entries, 1)
			attrs, ok := entries[0]["Attributes"].(map[string]any)
			require.True(t, ok)
			delete(attrs, "caller.full_path")
			assert.Equal(t, tt.want, attrs)
		})
	}
}
