package gcp

import (
	"context"
	"strconv"
	"testing"

	gcppubsub "cloud.google.com/go/pubsub/v2"
	"github.com/anthonycorbacho/workspace/kit/pubsub"
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

			attrs := map[string]string{"custom": "v"}
			tracingAttributes(span, attrs)
			assert.Equal(t, map[string]string{
				"custom":        "v",
				"trace":         sc.TraceID().String(),
				"span":          sc.SpanID().String(),
				"trace-state":   "vendor=value",
				"trace-remote":  "false",
				"trace-sampled": strconv.FormatBool(tt.sampled),
			}, attrs)

			got := trace.SpanContextFromContext(contextFromTracingAttributes(context.Background(), attrs))
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
		{name: "invalid trace id", attrs: with("trace", "not-hex")},
		{name: "zero trace id", attrs: with("trace", "00000000000000000000000000000000")},
		{name: "invalid span id", attrs: with("span", "xyz")},
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

func TestResourceName(t *testing.T) {
	tests := []struct {
		name     string
		project  string
		kind     string
		nameOrID string
		want     string
	}{
		{name: "topic id", project: "p", kind: "topics", nameOrID: "t", want: "projects/p/topics/t"},
		{name: "subscription id", project: "p", kind: "subscriptions", nameOrID: "s", want: "projects/p/subscriptions/s"},
		{name: "already qualified", project: "p", kind: "topics", nameOrID: "projects/other/topics/t", want: "projects/other/topics/t"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, resourceName(tt.project, tt.kind, tt.nameOrID))
		})
	}
}

func TestNilClient(t *testing.T) {
	p, err := NewPublisher(nil)
	assert.ErrorIs(t, err, pubsub.ErrNilClient)
	assert.Nil(t, p)

	s, err := NewSubscriber(nil)
	assert.ErrorIs(t, err, pubsub.ErrNilClient)
	assert.Nil(t, s)
}

func TestEmptyTopicAndSubscription(t *testing.T) {
	// the client is never used, the checks happen before reaching the server.
	c := &gcppubsub.Client{}
	p, err := NewPublisher(c)
	require.NoError(t, err)
	assert.ErrorIs(t, p.Publish(context.Background(), "", []byte("x")), pubsub.ErrNilTopic)

	s, err := NewSubscriber(c)
	require.NoError(t, err)
	assert.Error(t, s.Subscribe(context.Background(), "", func(context.Context, pubsub.Message, map[string]string) error {
		return nil
	}))
}
