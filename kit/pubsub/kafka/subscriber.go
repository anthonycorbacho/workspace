package kafka

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"sync"
	"time"

	"github.com/anthonycorbacho/workspace/kit/pubsub"
	"github.com/anthonycorbacho/workspace/kit/pubsub/internal/tracing"
	"github.com/twmb/franz-go/pkg/kgo"
	semconv "go.opentelemetry.io/otel/semconv/v1.43.0"
	"go.opentelemetry.io/otel/trace"
)

var _ pubsub.Subscriber = (*Subscriber)(nil)

// subscription is the handler of a subscription and the context of the records it handles.
type subscription struct {
	// ctx holds the values of the context given to SubscribeWithAck, not its cancellation:
	// the subscription lasts until the subscriber is closed.
	ctx     context.Context
	handler pubsub.HandlerWithAck
}

// regexHandler is a subscription to the topics matching a regex.
type regexHandler struct {
	re *regexp.Regexp
	subscription
}

// Subscriber consumes Kafka topics, the records are polled in the background until Close.
type Subscriber struct {
	closed    bool
	closeLock sync.RWMutex
	client    *Client
	ctx       context.Context
	cancel    context.CancelFunc

	muHandlerByTopic sync.RWMutex
	handlerByTopic   map[string]subscription

	muHandlerByRegex sync.RWMutex
	handlerByRegex   map[string]regexHandler

	rewinds *rewinder
}

// NewSubscriber creates a Kafka subscriber using client, it starts polling the records.
//
// Close must be called to stop polling and close the client.
func NewSubscriber(client *Client) (*Subscriber, error) {
	if client == nil {
		return nil, pubsub.ErrNilClient
	}

	// the poller lasts until Close, there is no caller context to derive it from.
	ctx, cancel := context.WithCancel(context.Background())
	s := &Subscriber{
		client:         client,
		ctx:            ctx,
		cancel:         cancel,
		handlerByTopic: make(map[string]subscription),
		handlerByRegex: make(map[string]regexHandler),
		rewinds:        newRewinder(),
	}
	go s.poller()

	return s, nil
}

// Close stops polling the records and closes the client.
func (s *Subscriber) Close() error {
	if !s.markClosed() {
		return nil
	}
	s.cancel()
	s.client.Close()
	return nil
}

// Subscribe subscribes to the subscription topic (or regex, see WithRegex).
//
// A record is acked once the handler succeeded, it is nacked when the handler failed (see SubscribeWithAck).
func (s *Subscriber) Subscribe(ctx context.Context, subscription string, handler pubsub.Handler) error {
	return s.SubscribeWithAck(ctx, subscription, pubsub.AckOnSuccess(handler))
}

// SubscribeWithAck subscribes to the subscription topic (or regex, see WithRegex).
//
// Kafka acknowledges the records of a partition in order: acking a record commits its offset
// (marks it with WithAutoCommit), acknowledging the previous records of the partition too.
// Nacking a record rewinds its partition: the record and the following ones are consumed again,
// the acks of the records of the partition delivered before the nack are ignored.
//
// The context of the handled records holds the values of ctx (eg: logger fields, baggage) but the
// subscription is not canceled with ctx, it lasts until the subscriber is closed.
func (s *Subscriber) SubscribeWithAck(ctx context.Context, subscription string, handler pubsub.HandlerWithAck) error {
	if s.isClosed() {
		return pubsub.ErrSubscriberClosed
	}

	if len(subscription) == 0 {
		return pubsub.ErrNilTopic
	}

	sub := newSubscription(ctx, handler)
	if s.client.regexEnabled {
		if err := s.registerHandlerByRegex(subscription, sub); err != nil {
			return err
		}
	} else {
		s.registerHandlerByTopic(subscription, sub)
		s.client.client.AddConsumeTopics(subscription)
	}

	return nil
}

func (s *Subscriber) poller() {
	for {
		select {
		case <-s.ctx.Done():
			return
		default:
			if s.client.regexEnabled && s.regexHandlerCount() < len(s.client.regexPatterns) {
				// Wait for all handlers to be registered
				time.Sleep(time.Second)
				continue
			}

			fetches := s.client.client.PollFetches(s.ctx)
			if fetches.IsClientClosed() {
				return
			}

			// Log partition errors but continue processing
			fetches.EachError(func(topic string, partition int32, err error) {
				_, span := tracer.Start(s.ctx, "subscription error "+topic, trace.WithAttributes(
					semconv.MessagingSystemKafka,
					semconv.MessagingDestinationName(topic),
					semconv.MessagingDestinationPartitionID(strconv.Itoa(int(partition))),
				))
				tracing.RecordError(span, err)
				span.End()
			})

			fetches.EachRecord(s.handleRecord)

			// set between the polls, it discards the records buffered after the rewound offsets.
			s.client.client.SetOffsets(s.rewinds.rewind())
		}
	}
}

// handleRecord handles record with the subscription of its topic.
func (s *Subscriber) handleRecord(record *kgo.Record) {
	generation, skip := s.rewinds.deliver(record)
	if skip {
		// the partition is rewound to a nacked record before it.
		return
	}

	sub, err := s.resolveHandlerForTopic(record.Topic)
	if err != nil {
		_, span := startProcessSpan(s.ctx, tracer, record)
		tracing.RecordError(span, err)
		span.End()
		return
	}

	ctx, span := startProcessSpan(sub.ctx, tracer, record)
	defer span.End()
	ctx = pubsub.WithTopic(ctx, record.Topic)

	// Create metadata from headers
	metadata := map[string]string{
		"topic":     record.Topic,
		"partition": strconv.Itoa(int(record.Partition)),
		"offset":    strconv.FormatInt(record.Offset, 10),
	}
	for _, h := range record.Headers {
		metadata[h.Key] = string(h.Value)
	}

	ack := func() {
		s.rewinds.ack(record, generation, func() {
			if s.client.autoCommitEnabled {
				s.client.client.MarkCommitRecords(record)
				return
			}
			if err := s.client.client.CommitRecords(ctx, record); err != nil {
				tracing.RecordError(span, err)
			}
		})
	}

	nack := func() {
		s.rewinds.nack(record, generation)
	}

	if err := sub.handler(ctx, record.Value, metadata, ack, nack); err != nil {
		tracing.RecordError(span, err)
	}
}

// newSubscription returns the subscription of handler, the records are handled with the values of ctx.
func newSubscription(ctx context.Context, handler pubsub.HandlerWithAck) subscription {
	return subscription{ctx: context.WithoutCancel(ctx), handler: handler}
}

func (s *Subscriber) registerHandlerByTopic(topic string, sub subscription) {
	s.muHandlerByTopic.Lock()
	defer s.muHandlerByTopic.Unlock()

	s.handlerByTopic[topic] = sub
}

func (s *Subscriber) registerHandlerByRegex(regex string, sub subscription) error {
	if _, ok := s.client.regexPatterns[regex]; !ok {
		return fmt.Errorf("regex %s is not configured in the subscriber", regex)
	}

	re, err := regexp.Compile(regex)
	if err != nil {
		return err
	}

	s.muHandlerByRegex.Lock()
	defer s.muHandlerByRegex.Unlock()

	s.handlerByRegex[regex] = regexHandler{
		re:           re,
		subscription: sub,
	}

	return nil
}

func (s *Subscriber) resolveHandlerForTopic(topic string) (subscription, error) {
	if s.client.regexEnabled {
		// Try to lookup the handler by topic first
		if h, err := s.lookupHandlerByTopic(topic); err == nil {
			return h, nil
		}

		// If no handler is found by topic, find the matching handler from the regexes
		h, err := s.matchHandlerByRegex(topic)
		if err != nil {
			return subscription{}, err
		}

		// Register the handler by topic to avoid future lookups
		s.registerHandlerByTopic(topic, h)

		return h, nil
	} else {
		return s.lookupHandlerByTopic(topic)
	}
}

func (s *Subscriber) regexHandlerCount() int {
	s.muHandlerByRegex.RLock()
	defer s.muHandlerByRegex.RUnlock()

	return len(s.handlerByRegex)
}

func (s *Subscriber) lookupHandlerByTopic(topic string) (subscription, error) {
	s.muHandlerByTopic.RLock()
	defer s.muHandlerByTopic.RUnlock()

	h, ok := s.handlerByTopic[topic]
	if !ok {
		return subscription{}, fmt.Errorf("handler not found for topic %s", topic)
	}

	return h, nil
}

func (s *Subscriber) matchHandlerByRegex(topic string) (subscription, error) {
	s.muHandlerByRegex.RLock()
	defer s.muHandlerByRegex.RUnlock()

	for _, rh := range s.handlerByRegex {
		if rh.re.MatchString(topic) {
			return rh.subscription, nil
		}
	}

	return subscription{}, fmt.Errorf("handler not found for topic %s", topic)
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
