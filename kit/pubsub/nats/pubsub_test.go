package nats

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/trace"
)

func testSpanContext(t *testing.T, sampled bool) trace.SpanContext {
	t.Helper()
	traceID, err := trace.TraceIDFromHex("0102030405060708090a0b0c0d0e0f10")
	require.NoError(t, err)
	spanID, err := trace.SpanIDFromHex("0102030405060708")
	require.NoError(t, err)
	state, err := trace.ParseTraceState("vendor=value")
	require.NoError(t, err)
	return trace.NewSpanContext(trace.SpanContextConfig{
		TraceID:    traceID,
		SpanID:     spanID,
		TraceState: state,
		TraceFlags: trace.TraceFlags(0).WithSampled(sampled),
	})
}

// firstValues flattens nats headers the same way the subscriber does.
func firstValues(h map[string][]string) map[string]string {
	m := make(map[string]string, len(h))
	for k, v := range h {
		m[k] = v[0]
	}
	return m
}

func TestTracingAttributesRoundTrip(t *testing.T) {
	tests := []struct {
		name    string
		sampled bool
	}{
		{name: "sampled", sampled: true},
		{name: "not sampled", sampled: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sc := testSpanContext(t, tt.sampled)
			span := trace.SpanFromContext(trace.ContextWithSpanContext(context.Background(), sc))

			headers := map[string][]string{}
			tracingAttributes(span, headers)
			assert.Equal(t, []string{sc.TraceID().String()}, headers["trace"])
			assert.Equal(t, []string{sc.SpanID().String()}, headers["span"])
			assert.Equal(t, []string{"vendor=value"}, headers["trace-state"])
			assert.Equal(t, []string{"false"}, headers["trace-remote"])

			got := trace.SpanContextFromContext(contextFromTracingAttributes(context.Background(), firstValues(headers)))
			require.True(t, got.IsValid())
			assert.True(t, got.IsRemote())
			assert.Equal(t, sc.TraceID(), got.TraceID())
			assert.Equal(t, sc.SpanID(), got.SpanID())
			assert.Equal(t, sc.TraceState().String(), got.TraceState().String())
			assert.Equal(t, tt.sampled, got.IsSampled(), "sampled flag should be propagated")
		})
	}
}

func TestContextFromTracingAttributesInvalid(t *testing.T) {
	valid := map[string]string{
		"trace":        "0102030405060708090a0b0c0d0e0f10",
		"span":         "0102030405060708",
		"trace-state":  "",
		"trace-remote": "false",
	}
	with := func(k, v string) map[string]string {
		m := map[string]string{}
		for key, val := range valid {
			m[key] = val
		}
		m[k] = v
		return m
	}
	tests := []struct {
		name  string
		attrs map[string]string
	}{
		{name: "nil", attrs: nil},
		{name: "empty", attrs: map[string]string{}},
		{name: "invalid trace id", attrs: with("trace", "not-hex")},
		{name: "zero trace id", attrs: with("trace", "00000000000000000000000000000000")},
		{name: "invalid span id", attrs: with("span", "xyz")},
		{name: "zero span id", attrs: with("span", "0000000000000000")},
		{name: "invalid trace state", attrs: with("trace-state", "no equal sign")},
		{name: "invalid remote flag", attrs: with("trace-remote", "maybe")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.WithValue(context.Background(), struct{}{}, "marker")
			got := contextFromTracingAttributes(ctx, tt.attrs)
			assert.Equal(t, ctx, got)
			assert.False(t, trace.SpanContextFromContext(got).IsValid())
		})
	}
}

func TestNewErrors(t *testing.T) {
	_, _, err := New("nats://127.0.0.1:1")
	assert.Error(t, err)
}
