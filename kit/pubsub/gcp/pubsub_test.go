package gcp

import (
	"context"
	"testing"

	gcppubsub "cloud.google.com/go/pubsub/v2"
	"github.com/anthonycorbacho/workspace/kit/pubsub"
	"github.com/anthonycorbacho/workspace/kit/pubsub/internal/tracingtest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/trace"
)

func TestTracePropagation(t *testing.T) {
	tests := []struct {
		name            string
		attributes      map[string]string
		wantDestination string
	}{
		{name: "published by a publisher", attributes: map[string]string{topicAttribute: "orders"}, wantDestination: "orders"},
		{name: "unknown topic", attributes: nil, wantDestination: "orders-sub"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := tracingtest.NewRecorder(t)
			msg := &gcppubsub.Message{Data: []byte("hello"), Attributes: tt.attributes, OrderingKey: "key"}

			_, publish := startPublishSpan(context.Background(), rec.Tracer(), tt.wantDestination, msg)
			publish.End()
			assert.Equal(t, publish.SpanContext().TraceID().String(), msg.Attributes["traceparent"][3:35])

			ctx, process := startProcessSpan(context.Background(), rec.Tracer(), "orders-sub", msg)
			process.End()
			assert.Equal(t, process.SpanContext(), trace.SpanContextFromContext(ctx))
			rec.AssertPropagated(t, tt.wantDestination, process)
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
