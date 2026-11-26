package nats

import (
	"context"
	"fmt"
	"strconv"
	"sync"
	"time"

	"github.com/anthonycorbacho/workspace/kit/errors"
	"github.com/anthonycorbacho/workspace/kit/log"
	"github.com/anthonycorbacho/workspace/kit/pubsub"
	"github.com/anthonycorbacho/workspace/kit/pubsub/internal/tracing"
	nats "github.com/nats-io/nats.go"
)

var _ pubsub.Subscriber = (*Subscriber)(nil)

// drainFlushGrace is added on top of the connection DrainTimeout to allow NATS's
// post-subscription publish FlushTimeout (5s) to finish before Close gives up.
const drainFlushGrace = 5 * time.Second

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
	backoff    pubsub.Backoff
}

// SubscriberOption configures a Subscriber.
type SubscriberOption func(*Subscriber)

// WithRetryBackoff sets the delay before redelivering a nacked message, pubsub.DefaultBackoff by default.
// The consumer MaxDeliver bounds the deliveries of a message, see the consumer configuration.
func WithRetryBackoff(b pubsub.Backoff) SubscriberOption {
	return func(s *Subscriber) {
		s.backoff = b
	}
}

// NewSubscriber creates a NATS JetStream subscriber using the connection natsClient and its
// JetStream context jetStreamCtx, the subscriptions are bound to consumer and join queueGroup.
//
// Close must be called to stop receiving messages and close the connection.
func NewSubscriber(queueGroup string, natsClient *nats.Conn, jetStreamCtx nats.JetStreamContext, consumer *nats.ConsumerInfo, opts ...SubscriberOption) (*Subscriber, error) {
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

	s := &Subscriber{
		closing:    make(chan struct{}, 1),
		closed:     false,
		closedLock: sync.Mutex{},
		queueGroup: queueGroup,
		nc:         natsClient,
		js:         jetStreamCtx,
		consumer:   consumer,
		backoff:    pubsub.DefaultBackoff,
	}
	for _, o := range opts {
		o(s)
	}
	return s, nil
}

// Close stops processing messages on all subscriptions, drains the connection,
// and waits for the connection to close within a bounded timeout.
func (s *Subscriber) Close() error {
	if !s.markClosed() {
		return nil
	}
	close(s.closing)

	if s.nc.IsClosed() {
		return pubsub.ErrSubscriberClosed
	}

	closedCh := s.nc.StatusChanged(nats.CLOSED)
	defer s.nc.RemoveStatusListener(closedCh)

	if err := s.nc.Drain(); err != nil {
		if errors.Is(err, nats.ErrConnectionClosed) {
			return pubsub.ErrSubscriberClosed
		}
		return err
	}

	if s.nc.IsClosed() {
		return s.nc.LastError()
	}

	timer := time.NewTimer(s.drainWaitTimeout())
	defer timer.Stop()
	select {
	case <-closedCh:
		return s.nc.LastError()
	case <-timer.C:
		s.nc.Close()
		return nats.ErrDrainTimeout
	}
}

func (s *Subscriber) drainWaitTimeout() time.Duration {
	timeout := s.nc.Opts.DrainTimeout
	if timeout <= 0 {
		timeout = nats.DefaultDrainTimeout
	}
	return timeout + drainFlushGrace
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
// A nacked message is redelivered after the retry backoff (see WithRetryBackoff), up to the consumer
// MaxDeliver, a message nacked with a permanent error (see pubsub.Permanent) is terminated: it is not
// redelivered. The delivery attempt is in the metadata (pubsub.MetadataDeliveryAttempt).
//
// The subscription is automatically unsubscribed when ctx is done, and any
// messages already buffered when ctx is done or the subscriber is closed are nacked.
func (s *Subscriber) SubscribeWithAck(ctx context.Context, subscription string /* subject */, handler pubsub.HandlerWithAck) error {
	if s.isClosed() || s.nc.IsClosed() {
		return pubsub.ErrSubscriberClosed
	}
	if len(subscription) == 0 {
		return fmt.Errorf("subscription is nil")
	}
	if err := ctx.Err(); err != nil {
		return err
	}

	subHandler := func(msg *nats.Msg) {
		s.receive(ctx, msg, handler)
	}

	sub, err := s.js.QueueSubscribe(
		subscription, /* subject */
		s.queueGroup,
		subHandler,
		nats.Bind(s.consumer.Stream, s.consumer.Name),
		nats.ManualAck())
	if err != nil {
		return fmt.Errorf("subscription init failed: %v", err)
	}

	// If Close raced with QueueSubscribe, remove the subscription immediately.
	if s.isClosed() {
		unsubscribeOnCancel(ctx, sub)
		return pubsub.ErrSubscriberClosed
	}

	s.watchSubscription(ctx, sub)
	return nil
}

// watchSubscription unsubscribes sub once ctx is canceled, stopping the server
// from continuing to deliver messages to a canceled handler context.
func (s *Subscriber) watchSubscription(ctx context.Context, sub *nats.Subscription) {
	if ctx.Done() == nil {
		return
	}
	go func() {
		select {
		case <-ctx.Done():
			unsubscribeOnCancel(ctx, sub)
		case <-s.closing:
		}
	}()
}

// unsubscribeOnCancel removes sub on context cancellation.
// Errors expected during concurrent shutdown or already-closed subscriptions are ignored.
func unsubscribeOnCancel(ctx context.Context, sub *nats.Subscription) {
	err := sub.Unsubscribe()
	if err == nil ||
		errors.Is(err, nats.ErrBadSubscription) ||
		errors.Is(err, nats.ErrConnectionClosed) ||
		errors.Is(err, nats.ErrConnectionDraining) {
		return
	}
	log.L().Warn(ctx, "failed to unsubscribe canceled subscription", log.Error(err))
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
	attempt := 1
	if md, err := msg.Metadata(); err == nil {
		attempt = int(md.NumDelivered)
	}
	metadata[pubsub.MetadataDeliveryAttempt] = strconv.Itoa(attempt)

	ack := func() {
		if err := msg.Ack(); err != nil {
			span.RecordError(err)
		}
	}
	nack := func(err error) {
		if pubsub.IsPermanent(err) {
			if termErr := msg.Term(); termErr != nil {
				span.RecordError(termErr)
			}
			return
		}
		if nakErr := msg.NakWithDelay(s.backoff.Delay(attempt)); nakErr != nil {
			span.RecordError(nakErr)
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
