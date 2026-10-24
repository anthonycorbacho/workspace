package nats

import (
	"context"
	"testing"

	"github.com/anthonycorbacho/workspace/kit/pubsub/internal/tracingtest"
	"github.com/nats-io/nats.go"
	"github.com/stretchr/testify/assert"
	"go.opentelemetry.io/otel/trace"
)

func TestTracePropagation(t *testing.T) {
	rec := tracingtest.NewRecorder(t)
	msg := &nats.Msg{Subject: "orders.created", Data: []byte("hello")}

	_, publish := startPublishSpan(context.Background(), rec.Tracer(), msg)
	publish.End()
	// the headers are case-sensitive, the key is not canonicalized.
	assert.Contains(t, msg.Header, "traceparent")

	ctx, process := startProcessSpan(context.Background(), rec.Tracer(), msg)
	process.End()
	assert.Equal(t, process.SpanContext(), trace.SpanContextFromContext(ctx))
	rec.AssertPropagated(t, "orders.created", process)
}

func TestHeaderCarrier(t *testing.T) {
	c := headerCarrier(nats.Header{"k": {"a", "b"}})
	assert.Equal(t, "a", c.Get("k"))
	assert.Equal(t, "", c.Get("K"), "keys are case-sensitive")
	c.Set("k", "c")
	c.Set("other", "d")
	assert.Equal(t, nats.Header{"k": {"c"}, "other": {"d"}}, nats.Header(c))
	assert.ElementsMatch(t, []string{"k", "other"}, c.Keys())
	assert.Equal(t, "", headerCarrier(nil).Get("k"))
}
