package pubsub

import "context"

// Publisher publishes a message to the given topic.
type Publisher interface {
	Publish(ctx context.Context, topic string, msg Message) error
}

// Subscriber subscribe to a topic subscription and handle the incoming event published to the topic.
type Subscriber interface {
	Subscribe(ctx context.Context, subscription string, handler Handler) error
	SubscribeWithAck(ctx context.Context, subscription string, handler HandlerWithAck) error
}

// Message is the message that is going to transit to the event pubsub.
type Message []byte

func (m Message) String() string {
	return string(m[:])
}

// Handler is the handler used to invoke the app handler.
type Handler func(ctx context.Context, msg Message) error

// HandlerWithAck is the handler used to invoke the app handler.
type HandlerWithAck func(ctx context.Context, msg Message, ack func(), nack func()) error

// Context type for topic
type topicCtxKeyType string

const topicCtxKey topicCtxKeyType = "topic"

// WithTopic inject to the given context the pubsub topic.
func WithTopic(ctx context.Context, topic string) context.Context {
	return context.WithValue(ctx, topicCtxKey, topic)
}

// GetTopic get the topic from the context.
// If the context doesnt have a topicCtxKey set,
// then the value returned will be an empty string.
func GetTopic(ctx context.Context) string {
	subject, ok := ctx.Value(topicCtxKey).(string)
	if !ok {
		return ""
	}
	return subject
}
