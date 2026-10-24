package kafka

import (
	"context"
	"sync"
	"time"

	"github.com/anthonycorbacho/workspace/kit/pubsub"
	"github.com/anthonycorbacho/workspace/kit/pubsub/internal/tracing"
	"github.com/twmb/franz-go/pkg/kgo"
)

var _ pubsub.Publisher = (*Publisher)(nil)

const (
	// publishTimeout is the maximum duration of a publish, unless the context is done before.
	publishTimeout = 5 * time.Second
	// closeTimeout is the maximum duration of the flush of the pending records on Close.
	closeTimeout = 10 * time.Second
)

// Publisher publishes a message on a Kafka topic.
type Publisher struct {
	closed    bool
	closeLock sync.RWMutex
	client    *Client
}

// NewPublisher creates a Kafka publisher using client.
//
// Close must be called to flush the pending records and close the client.
func NewPublisher(client *Client) (*Publisher, error) {
	if client == nil {
		return nil, pubsub.ErrNilClient
	}

	return &Publisher{
		client: client,
	}, nil
}

// Close stops publishing, flushes the pending records and closes the client.
func (p *Publisher) Close() error {
	p.closeLock.Lock()
	if p.closed {
		p.closeLock.Unlock()
		return nil
	}
	p.closed = true
	p.closeLock.Unlock()

	// Close has no context, the flush is bounded by closeTimeout.
	ctx, cancel := context.WithTimeout(context.Background(), closeTimeout)
	defer cancel()

	err := p.client.client.Flush(ctx)
	p.client.Close()
	return err
}

// Publish publishes a message on a Kafka topic.
// It blocks until the message is successfully published or an error occurred.
func (p *Publisher) Publish(ctx context.Context, topic string, msg pubsub.Message, opts ...pubsub.PublishOption) error {
	if len(topic) == 0 {
		return pubsub.ErrNilTopic
	}

	options := &pubsub.PublishOptions{
		Attributes: map[string]string{},
		Key:        "",
	}
	for _, o := range opts {
		o(options)
	}

	// the trace context is injected in the headers, it can't be overridden by the attributes of the options.
	headers := make([]kgo.RecordHeader, 0, len(options.Attributes)+3)
	for k, v := range options.Attributes {
		headers = append(headers, kgo.RecordHeader{Key: k, Value: []byte(v)})
	}
	record := &kgo.Record{
		Topic:   topic,
		Value:   msg,
		Headers: headers,
	}
	// a nil key lets the partitioner spread the messages, an empty key would send them all to the same partition.
	if options.Key != "" {
		record.Key = []byte(options.Key)
	}
	ctx, span := startPublishSpan(ctx, tracer, record)
	defer span.End()

	if p.isClose() {
		err := pubsub.ErrPublisherClosed
		tracing.RecordError(span, err)
		return err
	}

	// timeout for the publisher to give up and attempt to publish the message
	timeoutCtx, cancel := context.WithTimeout(ctx, publishTimeout)
	defer cancel()

	results := p.client.client.ProduceSync(timeoutCtx, record)
	if err := results.FirstErr(); err != nil {
		tracing.RecordError(span, err)
		return err
	}

	return nil
}

func (p *Publisher) isClose() bool {
	p.closeLock.RLock()
	defer p.closeLock.RUnlock()
	return p.closed
}
