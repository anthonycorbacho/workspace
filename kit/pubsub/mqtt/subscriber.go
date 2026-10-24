package mqtt

import (
	"context"
	"sync"

	"github.com/anthonycorbacho/workspace/kit/pubsub"
	"github.com/anthonycorbacho/workspace/kit/pubsub/internal/tracing"
	"github.com/eclipse/paho.golang/paho"
)

var _ pubsub.Subscriber = (*Subscriber)(nil)

// Subscriber subscribes to MQTT topics, the messages are received until Close.
type Subscriber struct {
	closed    bool
	closeLock sync.RWMutex
	client    *Client
}

// NewSubscriber creates a MQTT subscriber using client.
//
// Close must be called to disconnect the client.
func NewSubscriber(client *Client) (*Subscriber, error) {
	if client == nil {
		return nil, pubsub.ErrNilClient
	}

	return &Subscriber{
		client: client,
	}, nil
}

// Close stops receiving messages and disconnects the client.
func (s *Subscriber) Close() error {
	if !s.markClosed() {
		return nil
	}

	// Close has no context, the disconnection is bounded by disconnectTimeout.
	ctx, cancel := context.WithTimeout(context.Background(), disconnectTimeout)
	defer cancel()
	return s.client.Disconnect(ctx)
}

// Subscribe subscribes to the subscription topic (wildcards are supported).
//
// MQTT has no negative acknowledgement: the client acknowledges a message once the handler
// returned, a message the handler failed to process is not redelivered.
func (s *Subscriber) Subscribe(ctx context.Context, subscription string, handler pubsub.Handler) error {
	return s.SubscribeWithAck(ctx, subscription, pubsub.AckOnSuccess(handler))
}

// SubscribeWithAck subscribes to the subscription topic, ack and nack are no-ops (see Subscribe).
//
// ctx bounds the subscribe request, the messages are handled with the values of ctx (eg: logger
// fields, baggage) until the subscriber is closed.
func (s *Subscriber) SubscribeWithAck(ctx context.Context, subscription string, handler pubsub.HandlerWithAck) error {
	if s.isClosed() {
		return pubsub.ErrSubscriberClosed
	}

	if len(subscription) == 0 {
		return pubsub.ErrNilTopic
	}

	// the handlers are called until the client is disconnected, not until ctx is done.
	subCtx := context.WithoutCancel(ctx)
	s.client.route.RegisterHandler(subscription, func(msg *paho.Publish) {
		ctx, span := startProcessSpan(subCtx, tracer, msg)
		defer span.End()

		topic := msg.Properties.User.Get(topicProperty)
		if len(topic) == 0 {
			topic = msg.Topic
		}
		ctx = pubsub.WithTopic(ctx, topic)

		// create meta from userProp
		metadata := map[string]string{}
		metadata[topicProperty] = topic
		for _, p := range msg.Properties.User {
			metadata[p.Key] = p.Value
		}

		// MQTT has no concept of Ack and Nack, the client acknowledges the message once the handler returned.
		noop := func() {}
		if err := handler(ctx, msg.Payload, metadata, noop, noop); err != nil {
			tracing.RecordError(span, err)
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
