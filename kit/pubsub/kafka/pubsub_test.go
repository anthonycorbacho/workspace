package kafka

import (
	"context"
	"crypto/tls"
	"testing"

	"github.com/anthonycorbacho/workspace/kit/pubsub"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/twmb/franz-go/pkg/kgo"
	"go.opentelemetry.io/otel/trace"
)

// unreachableBroker is never dialed by the unit tests, kgo connects lazily.
const unreachableBroker = "127.0.0.1:1"

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

			existing := kgo.RecordHeader{Key: "custom", Value: []byte("v")}
			headers := tracingAttributes(span, []kgo.RecordHeader{existing})
			require.Len(t, headers, 6)
			assert.Equal(t, existing, headers[0])

			got := trace.SpanContextFromContext(contextFromTracingAttributes(context.Background(), headers))
			require.True(t, got.IsValid())
			assert.True(t, got.IsRemote())
			assert.Equal(t, sc.TraceID(), got.TraceID())
			assert.Equal(t, sc.SpanID(), got.SpanID())
			assert.Equal(t, sc.TraceState().String(), got.TraceState().String())
			assert.Equal(t, tt.sampled, got.IsSampled())
		})
	}
}

func TestContextFromTracingAttributesInvalid(t *testing.T) {
	valid := map[string]string{
		"trace":         "0102030405060708090a0b0c0d0e0f10",
		"span":          "0102030405060708",
		"trace-state":   "",
		"trace-remote":  "false",
		"trace-sampled": "true",
	}
	with := func(k, v string) []kgo.RecordHeader {
		var h []kgo.RecordHeader
		for key, val := range valid {
			if key == k {
				val = v
			}
			h = append(h, kgo.RecordHeader{Key: key, Value: []byte(val)})
		}
		return h
	}
	tests := []struct {
		name    string
		headers []kgo.RecordHeader
	}{
		{name: "nil", headers: nil},
		{name: "invalid trace id", headers: with("trace", "not-hex")},
		{name: "zero trace id", headers: with("trace", "00000000000000000000000000000000")},
		{name: "invalid span id", headers: with("span", "xyz")},
		{name: "invalid trace state", headers: with("trace-state", "no equal sign")},
		{name: "invalid remote flag", headers: with("trace-remote", "maybe")},
		{name: "invalid sampled flag", headers: with("trace-sampled", "maybe")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.WithValue(context.Background(), struct{}{}, "marker")
			got := contextFromTracingAttributes(ctx, tt.headers)
			assert.Equal(t, ctx, got)
			assert.False(t, trace.SpanContextFromContext(got).IsValid())
		})
	}
}

func TestNewClient(t *testing.T) {
	tests := []struct {
		name          string
		brokers       []string
		opts          []Option
		wantErr       bool
		wantAuto      bool
		wantRegex     bool
		wantRegexKeys []string
	}{
		{name: "no brokers", brokers: nil, wantErr: true},
		{name: "defaults", brokers: []string{unreachableBroker}},
		{name: "client id", brokers: []string{unreachableBroker}, opts: []Option{WithClientID("my-client")}},
		{
			name:     "consumer group with auto commit",
			brokers:  []string{unreachableBroker},
			opts:     []Option{WithConsumerGroup("group"), WithAutoCommit()},
			wantAuto: true,
		},
		{name: "consumer group manual commit", brokers: []string{unreachableBroker}, opts: []Option{WithConsumerGroup("group")}},
		{name: "tls", brokers: []string{unreachableBroker}, opts: []Option{WithTLS(&tls.Config{})}},
		{name: "sasl plain", brokers: []string{unreachableBroker}, opts: []Option{WithSASLPlain("user", "pass")}},
		{name: "sasl scram sha256", brokers: []string{unreachableBroker}, opts: []Option{WithSASLScramSHA256("user", "pass")}},
		{name: "sasl scram sha512", brokers: []string{unreachableBroker}, opts: []Option{WithSASLScramSHA512("user", "pass")}},
		{name: "auto topic creation", brokers: []string{unreachableBroker}, opts: []Option{WithAutoTopicCreation()}},
		{
			name:          "regex",
			brokers:       []string{unreachableBroker},
			opts:          []Option{WithRegex([]string{"^a.*", "^b.*", "^a.*"})},
			wantRegex:     true,
			wantRegexKeys: []string{"^a.*", "^b.*"},
		},
		{name: "invalid regex", brokers: []string{unreachableBroker}, opts: []Option{WithRegex([]string{"("})}, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, err := NewClient(tt.brokers, tt.opts...)
			if tt.wantErr {
				assert.Error(t, err)
				assert.Nil(t, c)
				return
			}
			require.NoError(t, err)
			defer c.Close()
			assert.Equal(t, tt.wantAuto, c.autoCommitEnabled)
			assert.Equal(t, tt.wantRegex, c.regexEnabled)
			keys := []string{}
			for k := range c.regexPatterns {
				keys = append(keys, k)
			}
			assert.ElementsMatch(t, tt.wantRegexKeys, keys)
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

func TestPublisherErrors(t *testing.T) {
	c, err := NewClient([]string{unreachableBroker})
	require.NoError(t, err)
	p, err := NewPublisher(c)
	require.NoError(t, err)

	assert.ErrorIs(t, p.Publish(context.Background(), "", []byte("x")), pubsub.ErrNilTopic)

	require.NoError(t, p.Close())
	assert.NoError(t, p.Close(), "closing twice is a no-op")
	assert.ErrorIs(t, p.Publish(context.Background(), "topic", []byte("x")), pubsub.ErrPublisherClosed)
}

func TestSubscriberErrors(t *testing.T) {
	noop := func(context.Context, pubsub.Message, map[string]string) error { return nil }
	tests := []struct {
		name         string
		opts         []Option
		subscription string
		wantErr      error
	}{
		{name: "empty subscription", subscription: "", wantErr: pubsub.ErrNilTopic},
		{name: "regex empty subscription", opts: []Option{WithRegex([]string{"^a.*"})}, subscription: "", wantErr: pubsub.ErrNilTopic},
		{name: "regex not configured", opts: []Option{WithRegex([]string{"^a.*"})}, subscription: "^b.*"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, err := NewClient([]string{unreachableBroker}, tt.opts...)
			require.NoError(t, err)
			s, err := NewSubscriber(c)
			require.NoError(t, err)
			defer s.Close()

			err = s.Subscribe(context.Background(), tt.subscription, noop)
			require.Error(t, err)
			if tt.wantErr != nil {
				assert.ErrorIs(t, err, tt.wantErr)
			}
		})
	}
}

func TestResolveHandlerForTopic(t *testing.T) {
	c, err := NewClient([]string{unreachableBroker}, WithRegex([]string{"^orders\\..*", "^users\\..*"}))
	require.NoError(t, err)
	s, err := NewSubscriber(c)
	require.NoError(t, err)
	defer s.Close()

	called := ""
	handlerFor := func(name string) pubsub.HandlerWithAck {
		return func(context.Context, pubsub.Message, map[string]string, func(), func()) error {
			called = name
			return nil
		}
	}
	require.NoError(t, s.SubscribeWithAck(context.Background(), "^orders\\..*", handlerFor("orders")))
	require.NoError(t, s.SubscribeWithAck(context.Background(), "^users\\..*", handlerFor("users")))

	tests := []struct {
		name    string
		topic   string
		want    string
		wantErr bool
	}{
		{name: "orders", topic: "orders.created", want: "orders"},
		{name: "orders cached", topic: "orders.created", want: "orders"},
		{name: "users", topic: "users.deleted", want: "users"},
		{name: "no match", topic: "payments.done", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			called = ""
			h, err := s.resolveHandlerForTopic(tt.topic)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.NoError(t, h(context.Background(), nil, nil, func() {}, func() {}))
			assert.Equal(t, tt.want, called)
		})
	}
}
