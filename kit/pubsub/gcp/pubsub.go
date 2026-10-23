package gcp

import (
	"context"
	"fmt"
	"strconv"
	"strings"

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
	m["trace-sampled"] = strconv.FormatBool(span.SpanContext().IsSampled())
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

	// the flag is written with strconv.FormatBool, a missing flag (eg: message published
	// by an older version) is considered not sampled.
	sampled := m["trace-sampled"] == "true"

	scc := trace.SpanContextConfig{
		TraceID:    traceID,
		SpanID:     spanID,
		TraceState: stats,
		TraceFlags: trace.TraceFlags(0).WithSampled(sampled),
		Remote:     remote,
	}

	sc := trace.NewSpanContext(scc)
	if !sc.IsValid() {
		return ctx
	}
	return trace.ContextWithRemoteSpanContext(ctx, sc)
}

// resourceName returns the fully qualified name of a pubsub resource (topics or subscriptions).
func resourceName(project, kind, nameOrID string) string {
	if strings.HasPrefix(nameOrID, "projects/") {
		return nameOrID
	}
	return fmt.Sprintf("projects/%s/%s/%s", project, kind, nameOrID)
}
