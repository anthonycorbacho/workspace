package mqtt

import (
	"context"
	"strconv"

	mqtt "github.com/eclipse/paho.golang/paho"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/trace"
)

// tracer represent a MQTT pubsub tracer
var tracer = otel.Tracer("kit/pubsub/mqtt")

// tracingAttributes returns the user properties with the tracing attributes added,
// properties are returned since UserProperties.Add appends to the slice.
func tracingAttributes(span trace.Span, m mqtt.UserProperties) mqtt.UserProperties {
	m.Add("trace", span.SpanContext().TraceID().String())
	m.Add("span", span.SpanContext().SpanID().String())
	m.Add("trace-state", span.SpanContext().TraceState().String())
	m.Add("trace-remote", strconv.FormatBool(span.SpanContext().IsRemote()))
	m.Add("trace-sampled", strconv.FormatBool(span.SpanContext().IsSampled()))
	return m
}

func contextFromTracingAttributes(ctx context.Context, m mqtt.UserProperties) context.Context {
	traceID, err := trace.TraceIDFromHex(m.Get("trace"))
	if err != nil {
		return ctx
	}
	spanID, err := trace.SpanIDFromHex(m.Get("span"))
	if err != nil {
		return ctx
	}

	stats, err := trace.ParseTraceState(m.Get("trace-state"))
	if err != nil {
		return ctx
	}

	remote, err := strconv.ParseBool(m.Get("trace-remote"))
	if err != nil {
		return ctx
	}

	// the flag is written with strconv.FormatBool, a missing flag (eg: message published
	// by an older version) is considered not sampled.
	sampled := m.Get("trace-sampled") == "true"

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
