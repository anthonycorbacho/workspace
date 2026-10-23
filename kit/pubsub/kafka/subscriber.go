package kafka

import (
	"context"
	"fmt"
	"regexp"
	"sync"
	"time"

	"github.com/anthonycorbacho/workspace/kit/pubsub"
	"github.com/twmb/franz-go/pkg/kgo"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

var _ pubsub.Subscriber = (*Subscriber)(nil)

type regexHandler struct {
	re      *regexp.Regexp
	handler pubsub.HandlerWithAck
}

// Subscriber attaches to a kafka subscription.
type Subscriber struct {
	closed    bool
	closeLock sync.RWMutex
	client    *Client
	ctx       context.Context
	cancel    context.CancelFunc

	muHandlerByTopic sync.RWMutex
	handlerByTopic   map[string]pubsub.HandlerWithAck

	muHandlerByRegex sync.RWMutex
	handlerByRegex   map[string]regexHandler
}

// NewSubscriber creates a new Kafka subscriber.
//
// It requires a call to Close in order to stop processing messages.
func NewSubscriber(client *Client) (*Subscriber, error) {
	if client == nil {
		return nil, pubsub.ErrNilClient
	}

	ctx, cancel := context.WithCancel(context.Background())
	s := &Subscriber{
		client:         client,
		ctx:            ctx,
		cancel:         cancel,
		handlerByTopic: make(map[string]pubsub.HandlerWithAck),
		handlerByRegex: make(map[string]regexHandler),
	}
	go s.poller()

	return s, nil
}

// Close notifies the Subscriber to stop processing messages and close the connection.
func (s *Subscriber) Close() error {
	if s.isClosed() {
		return nil
	}

	s.setClosed(true)
	s.cancel()
	s.client.Close()
	return nil
}

func (s *Subscriber) Subscribe(ctx context.Context, subscription string, handler pubsub.Handler) error {
	h := func(ctx context.Context, msg pubsub.Message, metadata map[string]string, ack func(), _ func()) error {
		// default behavior is to always ack.
		err := handler(ctx, msg, metadata)
		ack()
		return err
	}
	return s.SubscribeWithAck(ctx, subscription, h)
}

func (s *Subscriber) SubscribeWithAck(_ context.Context, subscription string, handler pubsub.HandlerWithAck) error {
	if s.isClosed() {
		return pubsub.ErrSubscriberClosed
	}

	if len(subscription) == 0 {
		return pubsub.ErrNilTopic
	}

	if s.client.regexEnabled {
		if err := s.registerHandlerByRegex(subscription, handler); err != nil {
			return err
		}
	} else {
		s.registerHandlerByTopic(subscription, handler)
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
				_, span := tracer.Start(s.ctx, fmt.Sprintf("subscription error %s", topic))
				span.SetAttributes(
					attribute.String("topic", topic),
					attribute.Int("partition", int(partition)),
				)
				span.RecordError(err)
				span.SetStatus(codes.Error, err.Error())
				span.End()
			})

			fetches.EachRecord(func(record *kgo.Record) {
				ctx := contextFromTracingAttributes(context.Background(), record.Headers)
				topic := record.Topic
				ctx = pubsub.WithTopic(ctx, topic)

				var span trace.Span
				ctx, span = tracer.Start(ctx, fmt.Sprintf("subscription %s", topic))
				defer span.End()

				handler, err := s.resolveHandlerForTopic(topic)
				if err != nil {
					span.RecordError(err)
					span.SetStatus(codes.Error, err.Error())
					return
				}

				span.SetAttributes(
					attribute.String("topic", topic),
					attribute.Int("partition", int(record.Partition)),
					attribute.Int64("offset", record.Offset),
				)

				// Create metadata from headers
				metadata := map[string]string{
					"topic":     topic,
					"partition": fmt.Sprintf("%d", record.Partition),
					"offset":    fmt.Sprintf("%d", record.Offset),
				}
				for _, h := range record.Headers {
					metadata[h.Key] = string(h.Value)
				}

				ack := func() {
					if s.client.autoCommitEnabled {
						s.client.client.MarkCommitRecords(record)
					} else {
						if err := s.client.client.CommitRecords(ctx, record); err != nil {
							span.RecordError(err)
							span.SetStatus(codes.Error, err.Error())
						}
					}
				}

				nack := func() {}

				if err := handler(ctx, record.Value, metadata, ack, nack); err != nil {
					span.RecordError(err)
					span.SetStatus(codes.Error, err.Error())
				}
			})
		}
	}
}

func (s *Subscriber) registerHandlerByTopic(topic string, handler pubsub.HandlerWithAck) {
	s.muHandlerByTopic.Lock()
	defer s.muHandlerByTopic.Unlock()

	s.handlerByTopic[topic] = handler
}

func (s *Subscriber) registerHandlerByRegex(regex string, handler pubsub.HandlerWithAck) error {
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
		re:      re,
		handler: handler,
	}

	return nil
}

func (s *Subscriber) resolveHandlerForTopic(topic string) (pubsub.HandlerWithAck, error) {
	if s.client.regexEnabled {
		// Try to lookup the handler by topic first
		if h, err := s.lookupHandlerByTopic(topic); err == nil {
			return h, nil
		}

		// If no handler is found by topic, find the matching handler from the regexes
		h, err := s.matchHandlerByRegex(topic)
		if err != nil {
			return nil, err
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

func (s *Subscriber) lookupHandlerByTopic(topic string) (pubsub.HandlerWithAck, error) {
	s.muHandlerByTopic.RLock()
	defer s.muHandlerByTopic.RUnlock()

	h, ok := s.handlerByTopic[topic]
	if !ok {
		return nil, fmt.Errorf("handler not found for topic %s", topic)
	}

	return h, nil
}

func (s *Subscriber) matchHandlerByRegex(topic string) (pubsub.HandlerWithAck, error) {
	s.muHandlerByRegex.RLock()
	defer s.muHandlerByRegex.RUnlock()

	for _, rh := range s.handlerByRegex {
		if rh.re.MatchString(topic) {
			return rh.handler, nil
		}
	}

	return nil, fmt.Errorf("handler not found for topic %s", topic)
}

func (s *Subscriber) setClosed(value bool) {
	s.closeLock.Lock()
	defer s.closeLock.Unlock()

	s.closed = value
}

func (s *Subscriber) isClosed() bool {
	s.closeLock.Lock()
	defer s.closeLock.Unlock()

	return s.closed
}
