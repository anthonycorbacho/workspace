package kafka

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/anthonycorbacho/workspace/kit/pubsub"
	"github.com/twmb/franz-go/pkg/kgo"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
)

var _ pubsub.Publisher = (*Publisher)(nil)

// Publisher publishes a message on a Kafka topic.
type Publisher struct {
	closed    bool
	closeLock sync.RWMutex
	client    *Client
}

// NewPublisher creates a new Kafka publisher.
//
// It requires a call to Close in order to stop processing messages.
func NewPublisher(client *Client) (*Publisher, error) {
	if client == nil {
		return nil, pubsub.ErrNilClient
	}

	return &Publisher{
		client: client,
	}, nil
}

// Close notifies the Publisher to stop processing messages, send all the remaining messages and close the connection.
func (p *Publisher) Close() error {
	p.closeLock.Lock()
	if p.closed {
		p.closeLock.Unlock()
		return nil
	}
	p.closed = true
	p.closeLock.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	p.client.client.Flush(ctx)
	p.client.Close()
	return nil
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

	ctx, span := tracer.Start(ctx, fmt.Sprintf("Publish %s", topic))
	span.SetAttributes(attribute.String("topic", topic))
	defer span.End()

	if p.isClose() {
		err := pubsub.ErrPublisherClosed
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return err
	}

	// Prepare headers with tracing and custom attributes
	headers := []kgo.RecordHeader{}
	headers = tracingAttributes(span, headers)

	for k, v := range options.Attributes {
		headers = append(headers, kgo.RecordHeader{
			Key:   k,
			Value: []byte(v),
		})
	}

	record := &kgo.Record{
		Key:     []byte(options.Key),
		Topic:   topic,
		Value:   msg,
		Headers: headers,
	}

	// timeout for the publisher to give up and attempt to publish the message
	timeoutCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	results := p.client.client.ProduceSync(timeoutCtx, record)
	if err := results.FirstErr(); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return err
	}

	return nil
}

func (p *Publisher) isClose() bool {
	p.closeLock.RLock()
	defer p.closeLock.RUnlock()
	return p.closed
}
