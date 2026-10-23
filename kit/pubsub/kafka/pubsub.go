package kafka

import (
	"context"
	"strconv"

	"github.com/twmb/franz-go/pkg/kgo"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/trace"
)

// tracer represents a Kafka pubsub tracer
var tracer = otel.Tracer("kit/pubsub/kafka")

func tracingAttributes(span trace.Span, headers []kgo.RecordHeader) []kgo.RecordHeader {
	headers = append(headers,
		kgo.RecordHeader{Key: "trace", Value: []byte(span.SpanContext().TraceID().String())},
		kgo.RecordHeader{Key: "span", Value: []byte(span.SpanContext().SpanID().String())},
		kgo.RecordHeader{Key: "trace-state", Value: []byte(span.SpanContext().TraceState().String())},
		kgo.RecordHeader{Key: "trace-remote", Value: []byte(strconv.FormatBool(span.SpanContext().IsRemote()))},
		kgo.RecordHeader{Key: "trace-sampled", Value: []byte(strconv.FormatBool(span.SpanContext().IsSampled()))},
	)
	return headers
}

func contextFromTracingAttributes(ctx context.Context, headers []kgo.RecordHeader) context.Context {
	headerMap := make(map[string]string)
	for _, h := range headers {
		headerMap[h.Key] = string(h.Value)
	}

	traceID, err := trace.TraceIDFromHex(headerMap["trace"])
	if err != nil {
		return ctx
	}
	spanID, err := trace.SpanIDFromHex(headerMap["span"])
	if err != nil {
		return ctx
	}

	stats, err := trace.ParseTraceState(headerMap["trace-state"])
	if err != nil {
		return ctx
	}

	remote, err := strconv.ParseBool(headerMap["trace-remote"])
	if err != nil {
		return ctx
	}

	sampled, err := strconv.ParseBool(headerMap["trace-sampled"])
	if err != nil {
		return ctx
	}

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
