// Package tracing propagates the trace context through the pubsub messages and starts the
// publish and process spans of the backends, following the OpenTelemetry messaging semantic
// conventions (https://opentelemetry.io/docs/specs/semconv/messaging/messaging-spans/).
//
// The trace context is written to and read from the messages with the global propagator
// (otel.GetTextMapPropagator), set by kit/telemetry.NewTracer to W3C TraceContext, Baggage and B3:
// the messages carry the traceparent, tracestate and baggage headers.
package tracing

import (
	"context"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/propagation"
	semconv "go.opentelemetry.io/otel/semconv/v1.43.0"
	"go.opentelemetry.io/otel/trace"
)

// Operation names of the messaging spans, the span names are "<operation> <destination>".
const (
	PublishOperation = "publish"
	ProcessOperation = "process"
)

// StartPublish starts the producer span of a message published to destination and injects its
// context into carrier, the headers of the message, so the subscribers continue the trace.
//
// system is the messaging.system attribute (eg: semconv.MessagingSystemKafka).
func StartPublish(
	ctx context.Context, tracer trace.Tracer, system attribute.KeyValue, destination string,
	carrier propagation.TextMapCarrier, attrs ...attribute.KeyValue,
) (context.Context, trace.Span) {
	ctx, span := tracer.Start(ctx, PublishOperation+" "+destination,
		trace.WithSpanKind(trace.SpanKindProducer),
		trace.WithAttributes(
			system,
			semconv.MessagingDestinationName(destination),
			semconv.MessagingOperationName(PublishOperation),
			semconv.MessagingOperationTypeSend,
		),
		trace.WithAttributes(attrs...),
	)
	otel.GetTextMapPropagator().Inject(ctx, carrier)
	return ctx, span
}

// StartProcess extracts the context injected by the publisher from carrier, the headers of the
// message, and starts the consumer span processing the message received from destination.
//
// The span is a child of the publish span, or a new root when the message carries no trace
// context: the span of ctx (eg: the span the subscription was created in) is not its parent.
// ctx values (eg: the baggage) are kept.
func StartProcess(
	ctx context.Context, tracer trace.Tracer, system attribute.KeyValue, destination string,
	carrier propagation.TextMapCarrier, attrs ...attribute.KeyValue,
) (context.Context, trace.Span) {
	ctx = otel.GetTextMapPropagator().Extract(ctx, carrier)
	opts := []trace.SpanStartOption{
		trace.WithSpanKind(trace.SpanKindConsumer),
		trace.WithAttributes(
			system,
			semconv.MessagingDestinationName(destination),
			semconv.MessagingOperationName(ProcessOperation),
			semconv.MessagingOperationTypeProcess,
		),
		trace.WithAttributes(attrs...),
	}
	if !trace.SpanContextFromContext(ctx).IsRemote() {
		opts = append(opts, trace.WithNewRoot())
	}
	return tracer.Start(ctx, ProcessOperation+" "+destination, opts...)
}

// RecordError records err on span and sets the span status to error.
func RecordError(span trace.Span, err error) {
	span.RecordError(err)
	span.SetStatus(codes.Error, err.Error())
}
