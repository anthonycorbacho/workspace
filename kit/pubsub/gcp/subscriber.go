package gcp

import (
	"context"
	"fmt"
	"sync"
	"time"

	gcppubsub "cloud.google.com/go/pubsub"
	"github.com/anthonycorbacho/workspace/kit/errors"
	"github.com/anthonycorbacho/workspace/kit/pubsub"
	"github.com/cenkalti/backoff/v4"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

var _ pubsub.Subscriber = (*Subscriber)(nil)

// SubscriberOption defines a Subscriber option.
type SubscriberOption func(*gcppubsub.ReceiveSettings)

// Subscriber attaches to a Google Cloud Pub/Sub subscription and returns a Go channel with messages from the topic.
// Be aware that in Google Cloud Pub/Sub, only messages sent after the subscription was created can be consumed.
//
// For more info on how Google Cloud Pub/Sub Subscribers work, check https://cloud.google.com/pubsub/docs/subscriber.
type Subscriber struct {
	closing                 chan struct{}
	closed                  bool
	closedLock              sync.Mutex
	subscriptionsWaitGroup  sync.WaitGroup
	activeSubscriptions     map[string]*gcppubsub.Subscription
	activeSubscriptionsLock sync.RWMutex
	client                  *gcppubsub.Client
	settings                gcppubsub.ReceiveSettings
}

// NewSubscriber creates a new GCP PubSub Subscriber.
//
// it required a call to Close in order to stop processing messages and close subscriber connections.
func NewSubscriber(client *gcppubsub.Client, opts ...SubscriberOption) (*Subscriber, error) {
	if client == nil {
		return nil, fmt.Errorf("pubsub client is nil")
	}

	// default receiveSettings
	settings := gcppubsub.ReceiveSettings{
		MaxExtension:           60 * time.Minute,
		MaxExtensionPeriod:     0,
		MinExtensionPeriod:     0,
		MaxOutstandingMessages: 1000,
		MaxOutstandingBytes:    1e9, // 1G
		NumGoroutines:          10,
	}
	for _, o := range opts {
		o(&settings)
	}

	return &Subscriber{
		closing:                 make(chan struct{}, 1),
		closed:                  false,
		closedLock:              sync.Mutex{},
		subscriptionsWaitGroup:  sync.WaitGroup{},
		activeSubscriptionsLock: sync.RWMutex{},
		activeSubscriptions:     map[string]*gcppubsub.Subscription{},
		client:                  client,
		settings:                settings,
	}, nil
}

// Close notifies the Subscriber to stop processing messages on all subscriptions, and terminate the connection.
func (s *Subscriber) Close() error {
	if s.isClosed() {
		return nil
	}
	s.setClosed(true)
	close(s.closing)

	// wait for all subscribers
	s.subscriptionsWaitGroup.Wait()

	return s.client.Close()
}

// Subscribe consumes Google Cloud Pub/Sub.
//
// In Google Cloud Pub/Sub, it is impossible to subscribe directly to a topic. Instead, a *subscription* is used.
// Each subscription has one topic, but there may be multiple subscriptions to one topic (with different names).
//
// Be aware that in Google Cloud Pub/Sub, only messages sent after the subscription was created can be consumed.
//
// See https://cloud.google.com/pubsub/docs/subscriber to find out more about how Google Cloud Pub/Sub Subscriptions work.
func (s *Subscriber) Subscribe(ctx context.Context, subscription string, handler pubsub.Handler) error {

	h := func(ctx context.Context, msg pubsub.Message, ack func(), nack func()) error {
		// default behavior is to always ack.
		ack()
		return handler(ctx, msg)
	}

	return s.SubscribeWithAck(ctx, subscription, h)
}

func (s *Subscriber) SubscribeWithAck(ctx context.Context, subscription string, handler pubsub.HandlerWithAck) error {
	if s.isClosed() {
		return fmt.Errorf("subscriber is closed")
	}

	if len(subscription) == 0 {
		return fmt.Errorf("subscription is nil")
	}

	ctx, cancelFn := context.WithCancel(ctx)
	sub, err := s.subscription(ctx, subscription)
	if err != nil {
		cancelFn()
		return err
	}

	// apply ReceiveSettings
	sub.ReceiveSettings = s.settings

	receiveFinished := make(chan struct{})
	s.subscriptionsWaitGroup.Add(1)
	go func(sub *gcppubsub.Subscription, handler pubsub.HandlerWithAck) {

		// utilise exponential Backoff on the subscription to give room to breeze.
		exponentialBackoff := backoff.NewExponentialBackOff()
		exponentialBackoff.MaxElapsedTime = 0 // 0 means it never expires

		if err := backoff.Retry(func() error {
			err := s.receive(ctx, sub, handler)
			if err == nil {
				// Receiving messages finished with no error
				return nil
			}

			// if the subscriber is closed, we will not retry anymore and exit.
			if s.isClosed() {
				return backoff.Permanent(err)
			}

			// Receiving messages failed, retrying
			return err
		}, exponentialBackoff); err != nil {
			// Retrying receiving messages failed
			fmt.Printf("retrying receiving messages failed: %s\n", err)
		}
		close(receiveFinished)
	}(sub, handler)

	// terminate the subscription
	go func(cancelFn context.CancelFunc) {
		<-s.closing
		cancelFn()
	}(cancelFn)

	go func() {
		<-receiveFinished
		s.subscriptionsWaitGroup.Done()
	}()

	return nil
}

func (s *Subscriber) receive(ctx context.Context, sub *gcppubsub.Subscription, handler pubsub.HandlerWithAck) error {
	err := sub.Receive(ctx, func(ctx context.Context, m *gcppubsub.Message) {

		select {
		case <-s.closing:
			m.Nack()
			return
		case <-ctx.Done():
			m.Nack()
			return
		default:
			// no-oop: responsibility of the caller
		}

		// recreate the context with traces
		ctx = contextFromTracingAttributes(ctx, m.Attributes)
		topic := m.Attributes["topic"]

		// Add to the context the topic.
		ctx = pubsub.WithTopic(ctx, topic)

		// annotate the span
		var span trace.Span
		ctx, span = tracer.Start(ctx, fmt.Sprintf("Subscription %s/%s", topic, sub.ID()))
		span.SetAttributes(attribute.String("subscription", sub.ID()))
		span.SetAttributes(attribute.String("topic", topic))
		defer span.End()

		ack := func() {
			m.Ack()
		}
		nack := func() {
			m.Nack()
		}

		// Process the message
		// in case of error, we record and label the error in the span.
		if err := handler(ctx, m.Data, ack, nack); err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
		}
	})
	return err
}

func (s *Subscriber) subscription(ctx context.Context, subscription string) (*gcppubsub.Subscription, error) {
	s.activeSubscriptionsLock.RLock()
	sub, ok := s.activeSubscriptions[subscription]
	s.activeSubscriptionsLock.RUnlock()
	if ok {
		return sub, nil
	}

	s.activeSubscriptionsLock.Lock()
	defer s.activeSubscriptionsLock.Unlock()

	sub = s.client.Subscription(subscription)
	exists, err := sub.Exists(ctx)
	if err != nil {
		return nil, errors.Wrapf(err, "could not check if subscription %s exists", subscription)
	}

	if !exists {
		return nil, errors.Wrap(errors.New("subscription does not exist"), subscription)
	}

	s.activeSubscriptions[subscription] = sub
	return sub, nil
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

// WithMaxExtension defines the maximum period for which the Subscription should
// automatically extend the ack deadline for each message.
//
// The Subscription will automatically extend the ack deadline of all
// fetched Messages up to the duration specified. Automatic deadline
// extension beyond the initial receipt may be disabled by specifying a
// duration less than 0.
func WithMaxExtension(d time.Duration) SubscriberOption {
	return func(o *gcppubsub.ReceiveSettings) {
		o.MaxExtension = d
	}
}

// WithMaxExtensionPeriod defines the maximum duration by which to extend the ack
// deadline at a time. The ack deadline will continue to be extended by up
// to this duration until MaxExtension is reached. Setting MaxExtensionPeriod
// bounds the maximum amount of time before a message redelivery in the
// event the subscriber fails to extend the deadline.
//
// MaxExtensionPeriod must be between 10s and 600s (inclusive). This configuration
// can be disabled by specifying a duration less than (or equal to) 0.
func WithMaxExtensionPeriod(d time.Duration) SubscriberOption {
	return func(o *gcppubsub.ReceiveSettings) {
		o.MaxExtensionPeriod = d
	}
}

// WithMinExtensionPeriod defines the min duration for a single lease extension attempt.
// By default the 99th percentile of ack latency is used to determine lease extension
// periods but this value can be set to minimize the number of extraneous RPCs sent.
//
// MinExtensionPeriod must be between 10s and 600s (inclusive). This configuration
// can be disabled by specifying a duration less than (or equal to) 0.
// Defaults to off but set to 60 seconds if the subscription has exactly-once delivery enabled,
// which will be added in a future release.
func WithMinExtensionPeriod(d time.Duration) SubscriberOption {
	return func(o *gcppubsub.ReceiveSettings) {
		o.MinExtensionPeriod = d
	}
}

// WithMaxOutstandingMessages defines the maximum number of unprocessed messages
// (unacknowledged but not yet expired). If MaxOutstandingMessages is 0, it
// will be treated as if it were DefaultReceiveSettings.MaxOutstandingMessages.
// If the value is negative, then there will be no limit on the number of
// unprocessed messages.
func WithMaxOutstandingMessages(n int) SubscriberOption {
	return func(o *gcppubsub.ReceiveSettings) {
		o.MaxOutstandingMessages = n
	}
}

// WithMaxOutstandingBytes defines the maximum size of unprocessed messages
// (unacknowledged but not yet expired). If MaxOutstandingBytes is 0, it will
// be treated as if it were DefaultReceiveSettings.MaxOutstandingBytes. If
// the value is negative, then there will be no limit on the number of bytes
// for unprocessed messages.
func WithMaxOutstandingBytes(n int) SubscriberOption {
	return func(o *gcppubsub.ReceiveSettings) {
		o.MaxOutstandingBytes = n
	}
}

// WithNumGoroutines defines the number of goroutines that each datastructure along
// the Receive path will spawn. Adjusting this value adjusts concurrency
// along the receive path.
//
// NumGoroutines defaults 10.
//
// NumGoroutines does not limit the number of messages that can be processed
// concurrently. Even with one goroutine, many messages might be processed at
// once, because that goroutine may continually receive messages and invoke the
// function passed to Receive on them. To limit the number of messages being
// processed concurrently, set MaxOutstandingMessages.
func WithNumGoroutines(n int) SubscriberOption {
	return func(o *gcppubsub.ReceiveSettings) {
		o.NumGoroutines = n
	}
}
