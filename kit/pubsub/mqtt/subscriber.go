package mqtt

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/anthonycorbacho/workspace/kit/pubsub"
	"github.com/eclipse/paho.golang/paho"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

var _ pubsub.Subscriber = (*Subscriber)(nil)

// Subscriber attaches to a mqtt subscription.
type Subscriber struct {
	closed    bool
	closeLock sync.RWMutex
	client    *Client
}

// NewSubscriber create a new MQTT subscriber.
//
// It required a call to Close in order to stop processing messages.
func NewSubscriber(client *Client) (*Subscriber, error) {
	if client == nil {
		return nil, pubsub.ErrNilClient
	}

	return &Subscriber{
		client: client,
	}, nil
}

// Close notifies the Subscriber to stop processing messages, send all the remaining messages and close the connection.
func (s *Subscriber) Close() error {
	if !s.markClosed() {
		return nil
	}

	ctx, fn := context.WithTimeout(context.Background(), 10*time.Second)
	defer fn()
	return s.client.Disconnect(ctx)
}

func (s *Subscriber) Subscribe(ctx context.Context, subscription string, handler pubsub.Handler) error {
	h := func(ctx context.Context, msg pubsub.Message, metadata map[string]string, ack func(), _ func()) error {
		// default behavior is to always ack.
		ack()
		return handler(ctx, msg, metadata)
	}
	return s.SubscribeWithAck(ctx, subscription, h)
}

func (s *Subscriber) SubscribeWithAck(ctx context.Context, subscription string, handler pubsub.HandlerWithAck) error {
	if s.isClosed() {
		return pubsub.ErrSubscriberClosed
	}

	if len(subscription) == 0 {
		return pubsub.ErrNilTopic
	}

	s.client.route.RegisterHandler(subscription, func(msg *paho.Publish) {
		ctx := contextFromTracingAttributes(context.Background(), msg.Properties.User)
		topic := msg.Properties.User.Get("topic")

		if len(topic) == 0 {
			topic = msg.Topic
		}

		// Add topic to the context
		ctx = pubsub.WithTopic(ctx, topic)

		var span trace.Span
		ctx, span = tracer.Start(ctx, fmt.Sprintf("subscription %s", topic))
		defer span.End()

		span.SetAttributes(attribute.String("topic", topic))

		// create meta from userProp
		metadata := map[string]string{}
		metadata["topic"] = topic
		for _, p := range msg.Properties.User {
			metadata[p.Key] = p.Value
		}

		// MQTT doest have concept of Ack and Nack.
		noop := func() {}
		if err := handler(ctx, msg.Payload, metadata, noop, noop); err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
		}
	})
	_, err := s.client.Subscribe(ctx, &paho.Subscribe{
		Subscriptions: []paho.SubscribeOptions{
			{
				Topic:          subscription,
				QoS:            2,
				RetainHandling: 2,
			},
		},
	})
	return err
}

// markClosed marks the subscriber as closed, it reports false if it was already closed
// so concurrent calls to Close only release the resources once.
func (s *Subscriber) markClosed() bool {
	s.closeLock.Lock()
	defer s.closeLock.Unlock()

	if s.closed {
		return false
	}
	s.closed = true
	return true
}

func (s *Subscriber) isClosed() bool {
	s.closeLock.Lock()
	defer s.closeLock.Unlock()

	return s.closed
}
