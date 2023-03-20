package gcp

import (
	"context"
	"strconv"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/trace"
)

// tracer represent a GCP pubsub tracer
var tracer = otel.Tracer("kit/pubsub/gcp")

func tracingAttributes(span trace.Span, m map[string]string) {

	m["trace"] = span.SpanContext().TraceID().String()
	m["span"] = span.SpanContext().SpanID().String()
	m["trace-state"] = span.SpanContext().TraceState().String()
	m["trace-remote"] = strconv.FormatBool(span.SpanContext().IsRemote())
}

func contextFromTracingAttributes(ctx context.Context, m map[string]string) context.Context {
	traceID, err := trace.TraceIDFromHex(m["trace"])
	if err != nil {
		return ctx
	}
	spanID, err := trace.SpanIDFromHex(m["span"])
	if err != nil {
		return ctx
	}

	stats, err := trace.ParseTraceState(m["trace-state"])
	if err != nil {
		return ctx
	}

	remote, err := strconv.ParseBool(m["trace-remote"])
	if err != nil {
		return ctx
	}

	scc := trace.SpanContextConfig{
		TraceID:    traceID,
		SpanID:     spanID,
		TraceState: stats,
		Remote:     remote,
	}

	sc := trace.NewSpanContext(scc)
	if !sc.IsValid() {
		return ctx
	}
	return trace.ContextWithRemoteSpanContext(ctx, sc)
}
