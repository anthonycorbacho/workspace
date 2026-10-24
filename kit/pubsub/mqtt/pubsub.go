package mqtt

import (
	"context"

	"github.com/anthonycorbacho/workspace/kit/pubsub/internal/tracing"
	"github.com/eclipse/paho.golang/paho"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	semconv "go.opentelemetry.io/otel/semconv/v1.43.0"
	"go.opentelemetry.io/otel/trace"
)

// topicProperty is the user property holding the topic a message is published to.
const topicProperty = "topic"

// tracer is the tracer of the MQTT publishers and subscribers.
var tracer = otel.Tracer("kit/pubsub/mqtt")

// system is the messaging.system attribute of the MQTT spans.
var system = semconv.MessagingSystemKey.String("mqtt")

// userPropertiesCarrier adapts the MQTT v5 user properties of a message to propagation.TextMapCarrier.
type userPropertiesCarrier struct {
	props *paho.UserProperties
}

var _ propagation.TextMapCarrier = userPropertiesCarrier{}

// Get returns the value of the first property with key.
func (c userPropertiesCarrier) Get(key string) string {
	return c.props.Get(key)
}

// Set sets the value of the property with key, replacing the properties with the same key.
func (c userPropertiesCarrier) Set(key, value string) {
	props := (*c.props)[:0]
	for _, p := range *c.props {
		if p.Key != key {
			props = append(props, p)
		}
	}
	*c.props = append(props, paho.UserProperty{Key: key, Value: value})
}

// Keys returns the keys of the properties.
func (c userPropertiesCarrier) Keys() []string {
	keys := make([]string, 0, len(*c.props))
	for _, p := range *c.props {
		keys = append(keys, p.Key)
	}
	return keys
}

// userProperties returns the user properties of msg, the properties are created when missing.
func userProperties(msg *paho.Publish) *paho.UserProperties {
	if msg.Properties == nil {
		msg.Properties = &paho.PublishProperties{}
	}
	return &msg.Properties.User
}

// startPublishSpan starts the span publishing msg, the span context is injected in the message user properties.
func startPublishSpan(ctx context.Context, tracer trace.Tracer, msg *paho.Publish) (context.Context, trace.Span) {
	return tracing.StartPublish(ctx, tracer, system, msg.Topic, userPropertiesCarrier{userProperties(msg)},
		semconv.MessagingMessageBodySize(len(msg.Payload)),
	)
}

// startProcessSpan starts the span processing msg, child of the span the message was published with.
func startProcessSpan(ctx context.Context, tracer trace.Tracer, msg *paho.Publish) (context.Context, trace.Span) {
	return tracing.StartProcess(ctx, tracer, system, msg.Topic, userPropertiesCarrier{userProperties(msg)},
		semconv.MessagingMessageBodySize(len(msg.Payload)),
	)
}
