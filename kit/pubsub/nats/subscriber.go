package nats

import (
	"context"
	"fmt"

	"github.com/anthonycorbacho/workspace/kit/errors"
	"github.com/anthonycorbacho/workspace/kit/pubsub"
	nats "github.com/nats-io/nats.go"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

var _ pubsub.Subscriber = (*Subscriber)(nil)

// Subscriber is our wrapper around NATS subscription.
// In current implementation, one Subscriber corresponds to one NATS subscription,
// as it's ok to have many subscriptions per client(https://docs.nats.io/using-nats/developer/anatomy#connecting-and-disconnecting)
//
// The following features are available our of the box:
// - automatic reconnection: https://docs.nats.io/using-nats/developer/connecting/reconnect
type Subscriber struct {
	// As we are likely to use the `Push + queue group` scenario
	// this design implies that a subscriber corresponds to a unique queue group
	// but may have multiple subscriptions
	queueGroup string
	// A consumer could be created by the action of subscribing,
	// but draining that subscription would also remove the consumer.
	// So for more control, it's better to create Consumers independently
	consumer *nats.ConsumerInfo
	nc       *nats.Conn
	js       nats.JetStreamContext
}

// NewSubscriber creates a new Nats Subscriber.
//
// it required a call to Close in order to stop processing messages and close subscriber connections.
func NewSubscriber(queueGroup string, natsClient *nats.Conn, jetStreamCtx nats.JetStreamContext, consumer *nats.ConsumerInfo) (*Subscriber, error) {
	if len(queueGroup) == 0 {
		return nil, errors.New("invalid queueGroup")
	}
	if natsClient == nil {
		return nil, errors.New("invalid nats client")
	}
	if jetStreamCtx == nil {
		return nil, errors.New("invalid nats jetstream")
	}
	if consumer == nil {
		return nil, errors.New("invalid nats consumer")
	}

	return &Subscriber{
		queueGroup: queueGroup,
		nc:         natsClient,
		js:         jetStreamCtx,
		consumer:   consumer,
	}, nil
}

// Close notifies the Subscriber to stop processing messages on all subscriptions, and terminate the connection.
//
// It is caller's responsibility to configure client's connection's `DrainTimeout` and `ClosedHandler` (with WaitGroup)
// https://docs.nats.io/using-nats/developer/receiving/drain
func (s *Subscriber) Close() error {
	if s.nc.IsClosed() {
		return pubsub.SubscriberCLosed
	}
	return s.nc.Drain()
}

// Subscribe consumes NATS Pub/Sub.
//
// NATS has two types of subscription: Pull and Push.
// ADA use-case seems to fit the `Push + queue group` scenario, which is why here we `QueueSubscribe“
//
// Read more about it https://docs.nats.io/reference/faq#what-is-the-right-kind-of-stream-consumer-to-use
//
// IMPORTANT! Don't forget to filter messages on the consumer as subscriber's subscription doesn't seem to take priority.
// Depending on the Consumer `DeliverPolicy`, `all`, `last`, `new`, `by_start_time`, `by_start_sequence`
// persisted messages can be received
func (s *Subscriber) Subscribe(ctx context.Context, subscription string /* subject */, handler pubsub.Handler) error {
	if s.nc.IsClosed() {
		return fmt.Errorf("subscriber is closed")
	}
	if len(subscription) == 0 {
		return fmt.Errorf("subscription is nil")
	}

	// exponential Backoff needed?
	_, err := s.js.QueueSubscribe(subscription /* subject */, s.queueGroup, func(msg *nats.Msg) {
		go s.receive(ctx, msg, handler)
	}, nats.Bind(s.consumer.Stream, s.consumer.Name))
	if err != nil {
		return fmt.Errorf("subscription init failed: %v", err)
	}

	return nil
}

func (s *Subscriber) SubscribeWithAck(ctx context.Context, subscription string /* subject */, handler pubsub.HandlerWithAck) error {
	return errors.New("not implemented")
}

func (s *Subscriber) receive(ctx context.Context, msg *nats.Msg, handler pubsub.Handler) {
	// recreate the context with traces
	firstHeaders := make(map[string]string)
	for k, v := range msg.Header {
		firstHeaders[k] = v[0]
	}
	ctx = contextFromTracingAttributes(ctx, firstHeaders)

	// Add to the context the topic (subject).
	ctx = pubsub.WithTopic(ctx, msg.Subject)

	// annotate the span
	var span trace.Span
	ctx, span = tracer.Start(ctx, fmt.Sprintf("Subscription %s", msg.Subject))
	span.SetAttributes(attribute.String("topic", msg.Subject))
	defer span.End()

	// Process the message
	// in case of error, we record and label the error in the span.
	if err := handler(ctx, msg.Data); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
	}
}
