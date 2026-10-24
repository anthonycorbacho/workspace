package nats

import (
	"context"

	"github.com/anthonycorbacho/workspace/kit/pubsub/internal/tracing"
	"github.com/nats-io/nats.go"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	semconv "go.opentelemetry.io/otel/semconv/v1.43.0"
	"go.opentelemetry.io/otel/trace"
)

// subjectHeader is the header holding the subject a message is published to.
const subjectHeader = "subject"

// tracer is the tracer of the NATS publishers and subscribers.
var tracer = otel.Tracer("kit/pubsub/nats")

// system is the messaging.system attribute of the NATS spans.
var system = semconv.MessagingSystemKey.String("nats")

// headerCarrier adapts the headers of a NATS message to propagation.TextMapCarrier.
//
// propagation.HeaderCarrier is not used as it canonicalizes the keys (eg: Traceparent) like
// http.Header while the NATS headers are case-sensitive, the keys are kept as is (eg: traceparent).
type headerCarrier nats.Header

var _ propagation.TextMapCarrier = headerCarrier{}

// Get returns the first value of key.
func (c headerCarrier) Get(key string) string {
	return nats.Header(c).Get(key)
}

// Set sets the value of key, replacing its values.
func (c headerCarrier) Set(key, value string) {
	nats.Header(c).Set(key, value)
}

// Keys returns the keys of the headers.
func (c headerCarrier) Keys() []string {
	keys := make([]string, 0, len(c))
	for k := range c {
		keys = append(keys, k)
	}
	return keys
}

// startPublishSpan starts the span publishing msg, the span context is injected in the message headers.
func startPublishSpan(ctx context.Context, tracer trace.Tracer, msg *nats.Msg) (context.Context, trace.Span) {
	if msg.Header == nil {
		msg.Header = nats.Header{}
	}
	return tracing.StartPublish(ctx, tracer, system, msg.Subject, headerCarrier(msg.Header),
		semconv.MessagingMessageBodySize(len(msg.Data)),
	)
}

// startProcessSpan starts the span processing msg, child of the span the message was published with.
func startProcessSpan(ctx context.Context, tracer trace.Tracer, msg *nats.Msg) (context.Context, trace.Span) {
	return tracing.StartProcess(ctx, tracer, system, msg.Subject, headerCarrier(msg.Header),
		semconv.MessagingMessageBodySize(len(msg.Data)),
	)
}
