package nats

import (
	"context"
	"fmt"
	"sync"

	"github.com/anthonycorbacho/workspace/kit/errors"
	"github.com/anthonycorbacho/workspace/kit/log"
	"github.com/anthonycorbacho/workspace/kit/pubsub"
	"github.com/anthonycorbacho/workspace/kit/pubsub/internal/tracing"
	nats "github.com/nats-io/nats.go"
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

// NewSubscriber creates a NATS JetStream subscriber using the connection natsClient and its
// JetStream context jetStreamCtx, the subscriptions are bound to consumer and join queueGroup.
//
// Close must be called to stop receiving messages and close the connection.
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
	if !s.markClosed() {
		return nil
	}
	close(s.closing)

	if s.nc.IsClosed() {
		return pubsub.ErrSubscriberClosed
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
//
// A message is acked once the handler succeeded, it is nacked and redelivered when the handler failed.
func (s *Subscriber) Subscribe(ctx context.Context, subscription string /* subject */, handler pubsub.Handler) error {
	return s.SubscribeWithAck(ctx, subscription, pubsub.AckOnSuccess(handler))
}

// SubscribeWithAck consumes the NATS subject like Subscribe, the handler acknowledges the messages.
// A message that is neither acked nor nacked is redelivered once the consumer AckWait expired.
//
// The messages received once ctx is done or the subscriber is closed are nacked.
func (s *Subscriber) SubscribeWithAck(ctx context.Context, subscription string /* subject */, handler pubsub.HandlerWithAck) error {
	if s.isClosed() || s.nc.IsClosed() {
		return pubsub.ErrSubscriberClosed
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
		nakOnClose(ctx, msg)
		return
	case <-ctx.Done():
		nakOnClose(ctx, msg)
		return
	default:
		// no-oop: responsibility of the caller
	}

	ctx, span := startProcessSpan(ctx, tracer, msg)
	defer span.End()
	// Add to the context the topic (subject).
	ctx = pubsub.WithTopic(ctx, msg.Subject)

	// the handler receives the first value of the headers.
	metadata := make(map[string]string, len(msg.Header))
	for k, v := range msg.Header {
		if len(v) > 0 {
			metadata[k] = v[0]
		}
	}

	ack := func() {
		if err := msg.Ack(); err != nil {
			span.RecordError(err)
		}
	}
	nack := func() {
		if err := msg.Nak(); err != nil {
			span.RecordError(err)
		}
	}

	// Process the message
	// in case of error, we record and label the error in the span.
	if err := handler(ctx, msg.Data, metadata, ack, nack); err != nil {
		tracing.RecordError(span, err)
	}
}

// markClosed marks the subscriber as closed, it reports false if it was already closed
// so concurrent calls to Close only release the resources once.
func (s *Subscriber) markClosed() bool {
	s.closedLock.Lock()
	defer s.closedLock.Unlock()

	if s.closed {
		return false
	}
	s.closed = true
	return true
}

func (s *Subscriber) isClosed() bool {
	s.closedLock.Lock()
	defer s.closedLock.Unlock()

	return s.closed
}

// nakOnClose negatively acknowledges a message received while the subscriber is closing,
// so it can be redelivered.
func nakOnClose(ctx context.Context, msg *nats.Msg) {
	if err := msg.Nak(); err != nil {
		log.L().Warn(ctx, "failed to nak message", log.Error(err))
	}
}
