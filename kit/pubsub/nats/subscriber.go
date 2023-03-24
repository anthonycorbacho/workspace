package nats

import (
	"context"
	"fmt"
	"sync"

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
	closing    chan struct{}
	closed     bool
	closedLock sync.Mutex
	queueGroup string
	consumer   *nats.ConsumerInfo
	nc         *nats.Conn
	js         nats.JetStreamContext
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
		closing:    make(chan struct{}, 1),
		closed:     false,
		closedLock: sync.Mutex{},
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
	if s.isClosed() {
		return nil
	}
	s.setClosed(true)
	close(s.closing)

	if s.nc.IsClosed() {
		return pubsub.SubscriberCLosed
	}
	return s.nc.Drain()
}

// Subscribe consumes NATS Pub/Sub.
//
// NATS has two types of subscription: Pull and Push.
//
// Read more about it https://docs.nats.io/reference/faq#what-is-the-right-kind-of-stream-consumer-to-use
//
// IMPORTANT! Don't forget to filter messages on the consumer as subscriber's subscription doesn't seem to take priority.
// Depending on the Consumer `DeliverPolicy`, `all`, `last`, `new`, `by_start_time`, `by_start_sequence`
// persisted messages can be received
func (s *Subscriber) Subscribe(ctx context.Context, subscription string /* subject */, handler pubsub.Handler) error {
	h := func(ctx context.Context, msg pubsub.Message, ack func(), nack func()) error {
		// default behavior is to always ack.
		ack()
		return handler(ctx, msg)
	}

	return s.SubscribeWithAck(ctx, subscription, h)
}

func (s *Subscriber) SubscribeWithAck(ctx context.Context, subscription string /* subject */, handler pubsub.HandlerWithAck) error {
	if s.nc.IsClosed() {
		return fmt.Errorf("subscriber is closed")
	}
	if len(subscription) == 0 {
		return fmt.Errorf("subscription is nil")
	}

	subHandler := func(msg *nats.Msg) {
		s.receive(ctx, msg, handler)
	}

	_, err := s.js.QueueSubscribe(
		subscription, /* subject */
		s.queueGroup,
		subHandler,
		nats.Bind(s.consumer.Stream, s.consumer.Name),
		nats.ManualAck())
	if err != nil {

		return fmt.Errorf("subscription init failed: %v", err)
	}

	return nil
}

func (s *Subscriber) receive(ctx context.Context, msg *nats.Msg, handler pubsub.HandlerWithAck) {

	select {
	case <-s.closing:
		msg.Nak()
		return
	case <-ctx.Done():
		msg.Nak()
		return
	default:
		// no-oop: responsibility of the caller
	}

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

	ack := func() {
		msg.Ack()
	}
	nack := func() {
		msg.Nak()
	}

	// Process the message
	// in case of error, we record and label the error in the span.
	err := handler(ctx, msg.Data, ack, nack)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
	}
}

func (s *Subscriber) setClosed(value bool) {
	s.closedLock.Lock()
	defer s.closedLock.Unlock()

	s.closed = value
}

func (s *Subscriber) isClosed() bool {
	s.closedLock.Lock()
	defer s.closedLock.Unlock()

	return s.closed
}
