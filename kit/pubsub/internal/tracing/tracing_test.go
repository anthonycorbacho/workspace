package tracing

import (
	"context"
	"errors"
	"testing"

	"github.com/anthonycorbacho/workspace/kit/pubsub/internal/tracingtest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/propagation"
	semconv "go.opentelemetry.io/otel/semconv/v1.43.0"
	"go.opentelemetry.io/otel/trace"
)

var system = semconv.MessagingSystemKey.String("test")

func TestStartPublishAndProcess(t *testing.T) {
	rec := tracingtest.NewRecorder(t)
	carrier := propagation.MapCarrier{}

	_, publish := StartPublish(context.Background(), rec.Tracer(), system, "orders", carrier, attribute.String("extra", "v"))
	publish.End()
	assert.Contains(t, carrier, "traceparent", "the W3C trace context is injected")

	ctx, process := StartProcess(context.Background(), rec.Tracer(), system, "orders", carrier)
	process.End()
	assert.Equal(t, process.SpanContext(), trace.SpanContextFromContext(ctx))
	rec.AssertPropagated(t, "orders", process)

	spans := rec.Ended()
	assert.Contains(t, spans[0].Attributes(), attribute.String("extra", "v"))
	for _, want := range []attribute.KeyValue{system, semconv.MessagingDestinationName("orders"), semconv.MessagingOperationTypeSend} {
		assert.Contains(t, spans[0].Attributes(), want)
	}
	for _, want := range []attribute.KeyValue{system, semconv.MessagingDestinationName("orders"), semconv.MessagingOperationTypeProcess} {
		assert.Contains(t, spans[1].Attributes(), want)
	}
}

func TestStartProcessWithoutTraceContext(t *testing.T) {
	rec := tracingtest.NewRecorder(t)

	// the subscription context span is not the parent of the messages processed.
	subCtx, subSpan := rec.Tracer().Start(context.Background(), "subscribe")
	defer subSpan.End()

	_, process := StartProcess(subCtx, rec.Tracer(), system, "orders", propagation.MapCarrier{})
	process.End()

	spans := rec.Ended()
	require.Len(t, spans, 1)
	assert.False(t, spans[0].Parent().IsValid(), "the span is a new root")
	assert.NotEqual(t, subSpan.SpanContext().TraceID(), spans[0].SpanContext().TraceID())
}

func TestRecordError(t *testing.T) {
	rec := tracingtest.NewRecorder(t)
	_, span := rec.Tracer().Start(context.Background(), "span")
	RecordError(span, errors.New("boom"))
	span.End()

	spans := rec.Ended()
	require.Len(t, spans, 1)
	assert.Equal(t, codes.Error, spans[0].Status().Code)
	assert.Equal(t, "boom", spans[0].Status().Description)
	require.Len(t, spans[0].Events(), 1)
	assert.Equal(t, "exception", spans[0].Events()[0].Name)
}
