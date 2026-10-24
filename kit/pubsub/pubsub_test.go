package pubsub

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCtx(t *testing.T) {
	ctx := WithTopic(context.Background(), "a.topic")
	topic := Topic(ctx)

	assert.Equal(t, "a.topic", topic)
}

func TestTopic(t *testing.T) {
	tests := []struct {
		name string
		ctx  context.Context
		want string
	}{
		{name: "no topic", ctx: context.Background(), want: ""},
		{name: "topic set", ctx: WithTopic(context.Background(), "orders"), want: "orders"},
		{name: "empty topic", ctx: WithTopic(context.Background(), ""), want: ""},
		{name: "overridden topic", ctx: WithTopic(WithTopic(context.Background(), "a"), "b"), want: "b"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, Topic(tt.ctx))
		})
	}
}

func TestMessageString(t *testing.T) {
	tests := []struct {
		name string
		msg  Message
		want string
	}{
		{name: "nil", msg: nil, want: ""},
		{name: "empty", msg: Message{}, want: ""},
		{name: "text", msg: Message("hello"), want: "hello"},
		{name: "utf8", msg: Message("こんにちは"), want: "こんにちは"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, tt.msg.String())
			assert.Equal(t, tt.want, fmt.Sprint(tt.msg))
		})
	}
}

func TestPublishOptions(t *testing.T) {
	attrs := map[string]string{"a": "1", "b": "2"}
	tests := []struct {
		name string
		opts []PublishOption
		want PublishOptions
	}{
		{name: "no options", opts: nil, want: PublishOptions{}},
		{name: "key", opts: []PublishOption{WithKey("k1")}, want: PublishOptions{Key: "k1"}},
		{name: "last key wins", opts: []PublishOption{WithKey("k1"), WithKey("k2")}, want: PublishOptions{Key: "k2"}},
		{name: "attributes", opts: []PublishOption{WithAttributes(attrs)}, want: PublishOptions{Attributes: attrs}},
		{name: "nil attributes", opts: []PublishOption{WithAttributes(nil)}, want: PublishOptions{}},
		{name: "qos 0", opts: []PublishOption{WithQoS(0)}, want: PublishOptions{QoS: 0}},
		{name: "qos 1", opts: []PublishOption{WithQoS(1)}, want: PublishOptions{QoS: 1}},
		{name: "qos 2", opts: []PublishOption{WithQoS(2)}, want: PublishOptions{QoS: 2}},
		{name: "qos negative clamps to 1", opts: []PublishOption{WithQoS(-1)}, want: PublishOptions{QoS: 1}},
		{name: "qos too high clamps to 1", opts: []PublishOption{WithQoS(3)}, want: PublishOptions{QoS: 1}},
		{name: "qos large clamps to 1", opts: []PublishOption{WithQoS(1 << 20)}, want: PublishOptions{QoS: 1}},
		{name: "retain", opts: []PublishOption{WithRetain()}, want: PublishOptions{Retained: true}},
		{
			name: "all options",
			opts: []PublishOption{WithKey("k"), WithAttributes(attrs), WithQoS(2), WithRetain()},
			want: PublishOptions{Key: "k", Attributes: attrs, QoS: 2, Retained: true},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := PublishOptions{}
			for _, o := range tt.opts {
				o(&got)
			}
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want string
	}{
		{name: "publisher closed", err: ErrPublisherClosed, want: "publisher is closed"},
		{name: "subscriber closed", err: ErrSubscriberClosed, want: "subscriber is closed"},
		{name: "nil topic", err: ErrNilTopic, want: "topic is nil"},
		{name: "nil client", err: ErrNilClient, want: "pubsub client is nil"},
		{name: "custom", err: Error("boom"), want: "boom"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.EqualError(t, tt.err, tt.want)
			wrapped := fmt.Errorf("wrapped: %w", tt.err)
			assert.ErrorIs(t, wrapped, tt.err)
			var pe Error
			assert.True(t, errors.As(wrapped, &pe))
			assert.Equal(t, tt.want, string(pe))
		})
	}
}

func TestAckOnSuccess(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		wantAck  bool
		wantNack bool
	}{
		{name: "handler succeeded", wantAck: true},
		{name: "handler failed", err: errors.New("failed"), wantNack: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var calls []string
			handler := func(ctx context.Context, msg Message, metadata map[string]string) error {
				calls = append(calls, "handler")
				assert.Equal(t, "orders", Topic(ctx))
				assert.Equal(t, Message("hello"), msg)
				assert.Equal(t, map[string]string{"k": "v"}, metadata)
				return tt.err
			}
			ack := func() { calls = append(calls, "ack") }
			nack := func() { calls = append(calls, "nack") }

			err := AckOnSuccess(handler)(WithTopic(context.Background(), "orders"), Message("hello"), map[string]string{"k": "v"}, ack, nack)
			assert.ErrorIs(t, err, tt.err)

			// the message is acknowledged once processed.
			want := []string{"handler"}
			if tt.wantAck {
				want = append(want, "ack")
			}
			if tt.wantNack {
				want = append(want, "nack")
			}
			assert.Equal(t, want, calls)
		})
	}
}
