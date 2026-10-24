package kafka

import (
	"context"
	"crypto/tls"
	"testing"

	"github.com/anthonycorbacho/workspace/kit/pubsub"
	"github.com/anthonycorbacho/workspace/kit/pubsub/internal/tracingtest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/twmb/franz-go/pkg/kgo"
	"go.opentelemetry.io/otel/trace"
)

// unreachableBroker is never dialed by the unit tests, kgo connects lazily.
const unreachableBroker = "127.0.0.1:1"

func TestTracePropagation(t *testing.T) {
	rec := tracingtest.NewRecorder(t)
	record := &kgo.Record{
		Topic:   "orders",
		Key:     []byte("key"),
		Value:   []byte("hello"),
		Headers: []kgo.RecordHeader{{Key: "custom", Value: []byte("v")}, {Key: "traceparent", Value: []byte("overridden")}},
	}

	_, publish := startPublishSpan(context.Background(), rec.Tracer(), record)
	publish.End()
	require.Len(t, record.Headers, 2, "the trace context replaces the traceparent header")
	assert.Equal(t, kgo.RecordHeader{Key: "custom", Value: []byte("v")}, record.Headers[0])

	ctx, process := startProcessSpan(context.Background(), rec.Tracer(), record)
	process.End()
	assert.Equal(t, process.SpanContext(), trace.SpanContextFromContext(ctx))
	rec.AssertPropagated(t, "orders", process)
}

func TestRecordCarrier(t *testing.T) {
	record := &kgo.Record{Headers: []kgo.RecordHeader{{Key: "k", Value: []byte("a")}, {Key: "k", Value: []byte("b")}}}
	c := recordCarrier{record}
	assert.Equal(t, "a", c.Get("k"))
	assert.Equal(t, "", c.Get("missing"))
	c.Set("k", "c")
	c.Set("other", "d")
	assert.Equal(t, []kgo.RecordHeader{{Key: "k", Value: []byte("c")}, {Key: "other", Value: []byte("d")}}, record.Headers)
	assert.Equal(t, []string{"k", "other"}, c.Keys())
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
			require.NoError(t, h.handler(h.ctx, nil, nil, func() {}, func() {}))
			assert.Equal(t, tt.want, called)
		})
	}
}

func TestSubscriptionContext(t *testing.T) {
	c, err := NewClient([]string{unreachableBroker})
	require.NoError(t, err)
	s, err := NewSubscriber(c)
	require.NoError(t, err)
	defer s.Close()

	type key struct{}
	ctx, cancel := context.WithCancel(context.WithValue(context.Background(), key{}, "value"))
	noop := func(context.Context, pubsub.Message, map[string]string, func(), func()) error { return nil }
	require.NoError(t, s.SubscribeWithAck(ctx, "orders", noop))
	cancel()

	sub, err := s.resolveHandlerForTopic("orders")
	require.NoError(t, err)
	assert.Equal(t, "value", sub.ctx.Value(key{}), "the records are handled with the subscription context values")
	assert.NoError(t, sub.ctx.Err(), "the subscription lasts until Close")
}
