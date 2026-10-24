package pubsub

import (
	"context"
)

// Publisher publishes messages to topics.
type Publisher interface {
	// Publish publishes msg to topic, it blocks until the backend acknowledged the message or ctx is done.
	// The trace context of ctx is propagated to the subscribers through the message headers.
	Publish(ctx context.Context, topic string, msg Message, opts ...PublishOption) error
}

// Subscriber subscribes to topic subscriptions and handles the messages published to the topics.
type Subscriber interface {
	// Subscribe acks a message once the handler succeeded and nacks it when the handler failed,
	// so it is redelivered when the backend supports it (see AckOnSuccess).
	Subscribe(ctx context.Context, subscription string, handler Handler) error
	// SubscribeWithAck leaves the acknowledgement of the messages to the handler.
	SubscribeWithAck(ctx context.Context, subscription string, handler HandlerWithAck) error
}

// Message is the message that is going to transit to the event pubsub.
type Message []byte

// String returns the message as a string.
func (m Message) String() string {
	return string(m)
}

// Handler handles a message received by a subscription.
//
// ctx holds the process span of the message, child of the publish span, and the topic (see Topic).
// metadata holds the message headers (attributes, user properties...), including the trace
// context headers (eg: traceparent) and the headers added by the backend (eg: topic).
type Handler func(ctx context.Context, msg Message, metadata map[string]string) error

// HandlerWithAck handles a message received by a subscription, like Handler, and acknowledges it:
// ack acknowledges the message, nack asks the backend to redeliver it when it supports it.
type HandlerWithAck func(ctx context.Context, msg Message, metadata map[string]string, ack func(), nack func()) error

// AckOnSuccess returns a HandlerWithAck acking the message once handler succeeded and nacking it
// when handler failed, acking before would lose the messages the handler failed to process.
func AckOnSuccess(handler Handler) HandlerWithAck {
	return func(ctx context.Context, msg Message, metadata map[string]string, ack func(), nack func()) error {
		if err := handler(ctx, msg, metadata); err != nil {
			nack()
			return err
		}
		ack()
		return nil
	}
}

// topicCtxKeyType is the type of the context key of the topic.
type topicCtxKeyType string

const topicCtxKey topicCtxKeyType = "topic"

// WithTopic returns a copy of ctx holding the pubsub topic, the subscribers set the topic of
// the message handled.
func WithTopic(ctx context.Context, topic string) context.Context {
	return context.WithValue(ctx, topicCtxKey, topic)
}

// Topic returns the pubsub topic held by ctx (see WithTopic), or an empty string.
func Topic(ctx context.Context) string {
	topic, _ := ctx.Value(topicCtxKey).(string)
	return topic
}

// PublishOptions are the options of a published message, they are set with the PublishOption.
// The backends ignore the options they don't support.
type PublishOptions struct {
	// Key is the message key: the ordering key on Google Cloud Pub/Sub, the record key on Kafka.
	Key string
	// Attributes are the message headers (attributes, user properties...), the headers set by
	// the backend (eg: topic, traceparent) can't be overridden.
	Attributes map[string]string
	// QoS is the MQTT quality of service, between 0 and 2.
	QoS int
	// Retained reports whether the MQTT broker retains the message.
	Retained bool
}

// PublishOption defines a Publish Option.
type PublishOption func(*PublishOptions)

// WithKey defines publish message key.
func WithKey(opts string) PublishOption {
	return func(po *PublishOptions) {
		po.Key = opts
	}
}

// WithAttributes defines publish attributes.
func WithAttributes(opts map[string]string) PublishOption {
	return func(po *PublishOptions) {
		po.Attributes = opts
	}
}

// WithQoS defines publish Quality of Service option (MQTT), an invalid QoS falls back to 1.
func WithQoS(opts int) PublishOption {
	return func(po *PublishOptions) {
		if opts < 0 || opts > 2 {
			opts = 1
		}
		po.QoS = opts
	}
}

// WithRetain asks the broker to retain the message (MQTT).
func WithRetain() PublishOption {
	return func(po *PublishOptions) {
		po.Retained = true
	}
}
