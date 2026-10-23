package mqtt

import (
	"context"
	"crypto/tls"
	"testing"
	"time"

	"github.com/anthonycorbacho/workspace/kit/pubsub"
	"github.com/eclipse/paho.golang/paho"
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

			props := paho.UserProperties{}
			props = tracingAttributes(span, props)
			assert.Equal(t, sc.TraceID().String(), props.Get("trace"))
			assert.Equal(t, sc.SpanID().String(), props.Get("span"))
			assert.Equal(t, "vendor=value", props.Get("trace-state"))
			assert.Equal(t, "false", props.Get("trace-remote"))

			got := trace.SpanContextFromContext(contextFromTracingAttributes(context.Background(), props))
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
	with := func(k, v string) paho.UserProperties {
		props := paho.UserProperties{}
		for key, val := range valid {
			if key == k {
				val = v
			}
			props.Add(key, val)
		}
		return props
	}
	tests := []struct {
		name  string
		props paho.UserProperties
	}{
		{name: "nil", props: nil},
		{name: "invalid trace id", props: with("trace", "not-hex")},
		{name: "zero trace id", props: with("trace", "00000000000000000000000000000000")},
		{name: "invalid span id", props: with("span", "xyz")},
		{name: "invalid trace state", props: with("trace-state", "no equal sign")},
		{name: "invalid remote flag", props: with("trace-remote", "maybe")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.WithValue(context.Background(), struct{}{}, "marker")
			got := contextFromTracingAttributes(ctx, tt.props)
			assert.Equal(t, ctx, got)
			assert.False(t, trace.SpanContextFromContext(got).IsValid())
		})
	}
}

func TestOptions(t *testing.T) {
	tlsConfig := &tls.Config{}
	tests := []struct {
		name string
		opt  Option
		want config
	}{
		{name: "retained", opt: EnableRetained(), want: config{retained: true}},
		{name: "keep alive", opt: WithKeepAlive(42), want: config{keepAlive: 42}},
		{name: "retry", opt: WithRetry(time.Minute), want: config{connectRetryDelay: time.Minute}},
		{name: "user", opt: WithUser("bob"), want: config{user: "bob"}},
		{name: "password", opt: WithPassword("secret"), want: config{password: "secret"}},
		{name: "tls", opt: WithTLS(tlsConfig), want: config{tls: tlsConfig}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := config{}
			tt.opt(&got)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestNewClientValidation(t *testing.T) {
	tests := []struct {
		name     string
		address  string
		clientID string
	}{
		{name: "empty address", address: "", clientID: "id"},
		{name: "invalid address", address: "not a url", clientID: "id"},
		{name: "empty client id", address: "mqtt://127.0.0.1:1883", clientID: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, err := NewClient(tt.address, tt.clientID)
			assert.Error(t, err)
			assert.Nil(t, c)
		})
	}
}

func TestNewClientUnreachable(t *testing.T) {
	c, err := NewClient("mqtt://127.0.0.1:1", "id", WithRetry(100*time.Millisecond))
	assert.Error(t, err)
	assert.Nil(t, c)
}

func TestNilClient(t *testing.T) {
	p, err := NewPublisher(nil)
	assert.ErrorIs(t, err, pubsub.ErrNilClient)
	assert.Nil(t, p)

	s, err := NewSubscriber(nil)
	assert.ErrorIs(t, err, pubsub.ErrNilClient)
	assert.Nil(t, s)
}

func TestPublisherErrors(t *testing.T) {
	// the client is never used, the checks happen before reaching the broker.
	p, err := NewPublisher(&Client{})
	require.NoError(t, err)
	assert.ErrorIs(t, p.Publish(context.Background(), "", []byte("x")), pubsub.ErrNilTopic)
}
