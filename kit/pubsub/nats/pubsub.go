package nats

import (
	"context"
	"strconv"

	"github.com/nats-io/nats.go"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/trace"
)

// tracer represents a NATS pubsub tracer
var tracer = otel.Tracer("kit/pubsub/nats")

func tracingAttributes(span trace.Span, m map[string][]string) {

	m["trace"] = []string{span.SpanContext().TraceID().String()}
	m["span"] = []string{span.SpanContext().SpanID().String()}
	m["trace-state"] = []string{span.SpanContext().TraceState().String()}
	m["trace-remote"] = []string{strconv.FormatBool(span.SpanContext().IsRemote())}
	m["trace-sampled"] = []string{strconv.FormatBool(span.SpanContext().IsSampled())}
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

// New returns JetStream context, nats connection and an error.
// Since we are most likely to only be using JetStream,
// there is no need to separate the initialization of NatsConn and JetStream
func New(url string, options ...func(*nats.Conn)) (nats.JetStreamContext, *nats.Conn, error) {
	nc, err := nats.Connect(url)
	if err != nil {
		return nil, nil, err
	}
	for _, option := range options {
		option(nc)
	}
	// Create JetStream Context
	js, err := nc.JetStream(nats.PublishAsyncMaxPending(256))
	if err != nil {
		nc.Close()
		return nil, nil, err
	}
	return js, nc, nil
}
