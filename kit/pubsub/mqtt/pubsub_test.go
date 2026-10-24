package mqtt

import (
	"context"
	"crypto/tls"
	"testing"
	"time"

	"github.com/anthonycorbacho/workspace/kit/pubsub"
	"github.com/anthonycorbacho/workspace/kit/pubsub/internal/tracingtest"
	"github.com/eclipse/paho.golang/paho"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/trace"
)

func TestTracePropagation(t *testing.T) {
	rec := tracingtest.NewRecorder(t)
	msg := &paho.Publish{Topic: "kit/orders", Payload: []byte("hello")}

	_, publish := startPublishSpan(context.Background(), rec.Tracer(), msg)
	publish.End()
	assert.NotEmpty(t, msg.Properties.User.Get("traceparent"))

	ctx, process := startProcessSpan(context.Background(), rec.Tracer(), msg)
	process.End()
	assert.Equal(t, process.SpanContext(), trace.SpanContextFromContext(ctx))
	rec.AssertPropagated(t, "kit/orders", process)
}

func TestUserPropertiesCarrier(t *testing.T) {
	props := paho.UserProperties{{Key: "k", Value: "a"}, {Key: "k", Value: "b"}}
	c := userPropertiesCarrier{&props}
	assert.Equal(t, "a", c.Get("k"))
	assert.Equal(t, "", c.Get("missing"))
	c.Set("k", "c")
	c.Set("other", "d")
	assert.Equal(t, paho.UserProperties{{Key: "k", Value: "c"}, {Key: "other", Value: "d"}}, props)
	assert.Equal(t, []string{"k", "other"}, c.Keys())

	// a message received without properties.
	_, span := startProcessSpan(context.Background(), tracingtest.NewRecorder(t).Tracer(), &paho.Publish{Topic: "t"})
	span.End()
}

func TestOptions(t *testing.T) {
	tlsConfig := &tls.Config{}
	tests := []struct {
		name string
		opt  Option
		want config
	}{
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
			c, err := NewClient(context.Background(), tt.address, tt.clientID)
			assert.Error(t, err)
			assert.Nil(t, c)
		})
	}
}

func TestNewClientUnreachable(t *testing.T) {
	c, err := NewClient(context.Background(), "mqtt://127.0.0.1:1", "id", WithRetry(100*time.Millisecond))
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
