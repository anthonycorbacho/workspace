package gcp

import (
	"context"
	"fmt"
	"strings"

	gcppubsub "cloud.google.com/go/pubsub/v2"
	"github.com/anthonycorbacho/workspace/kit/pubsub/internal/tracing"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/propagation"
	semconv "go.opentelemetry.io/otel/semconv/v1.43.0"
	"go.opentelemetry.io/otel/trace"
)

// topicAttribute is the attribute holding the topic a message is published to.
const topicAttribute = "topic"

// tracer is the tracer of the Google Cloud Pub/Sub publishers and subscribers.
var tracer = otel.Tracer("kit/pubsub/gcp")

// startPublishSpan starts the span publishing msg to topic, the span context is injected in the
// message attributes.
func startPublishSpan(ctx context.Context, tracer trace.Tracer, topic string, msg *gcppubsub.Message) (context.Context, trace.Span) {
	if msg.Attributes == nil {
		msg.Attributes = map[string]string{}
	}
	attrs := []attribute.KeyValue{semconv.MessagingMessageBodySize(len(msg.Data))}
	if msg.OrderingKey != "" {
		attrs = append(attrs, semconv.MessagingGCPPubSubMessageOrderingKey(msg.OrderingKey))
	}
	return tracing.StartPublish(ctx, tracer, semconv.MessagingSystemGCPPubSub, topic, propagation.MapCarrier(msg.Attributes), attrs...)
}

// startProcessSpan starts the span processing msg received from subscription, child of the
// span the message was published with.
func startProcessSpan(ctx context.Context, tracer trace.Tracer, subscription string, msg *gcppubsub.Message) (context.Context, trace.Span) {
	// the topic is unknown when the message was not published by a Publisher.
	destination := msg.Attributes[topicAttribute]
	if destination == "" {
		destination = subscription
	}
	return tracing.StartProcess(ctx, tracer, semconv.MessagingSystemGCPPubSub, destination, propagation.MapCarrier(msg.Attributes),
		semconv.MessagingDestinationSubscriptionName(subscription),
		semconv.MessagingMessageID(msg.ID),
		semconv.MessagingMessageBodySize(len(msg.Data)),
	)
}

// resourceName returns the fully qualified name of a pubsub resource (topics or subscriptions).
func resourceName(project, kind, nameOrID string) string {
	if strings.HasPrefix(nameOrID, "projects/") {
		return nameOrID
	}
	return fmt.Sprintf("projects/%s/%s/%s", project, kind, nameOrID)
}
