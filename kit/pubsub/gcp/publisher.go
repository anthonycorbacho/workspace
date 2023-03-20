package gcp

import (
	"context"
	"fmt"
	"sync"
	"time"

	gcppubsub "cloud.google.com/go/pubsub"
	"github.com/anthonycorbacho/workspace/kit/errors"
	"github.com/anthonycorbacho/workspace/kit/pubsub"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

var _ pubsub.Publisher = (*Publisher)(nil)

// Publisher publishes a message on a Google Cloud Pub/Sub topic.
//
// For more info on how Google Cloud Pub/Sub Publisher work, check https://cloud.google.com/pubsub/docs/publisher.
type Publisher struct {
	topics     map[string]*gcppubsub.Topic
	topicsLock sync.RWMutex
	closed     bool
	closeLock  sync.RWMutex
	client     *gcppubsub.Client
}

// NewPublisher create a new GCP publisher.
//
// It required a call to Close in order to stop processing messages and close topic connections.
func NewPublisher(client *gcppubsub.Client) (*Publisher, error) {
	if client == nil {
		return nil, fmt.Errorf("pubsub client is nil")
	}

	return &Publisher{
		topics: map[string]*gcppubsub.Topic{},
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

	p.topicsLock.Lock()
	for _, t := range p.topics {
		t.Stop()
	}
	p.topicsLock.Unlock()

	return p.client.Close()
}

// Publish publishes a message on a Google Cloud Pub/Sub topic.
// It blocks until the message is successfully published or an error occurred.
//
// To receive messages published to a topic, you must create a subscription to that topic.
// Only messages published to the topic after the subscription is created are available to subscriber applications.
//
// See https://cloud.google.com/pubsub/docs/publisher to find out more about how Google Cloud Pub/Sub Publishers work.
func (p *Publisher) Publish(ctx context.Context, topic string, msg pubsub.Message) error {
	if len(topic) == 0 {
		return fmt.Errorf("topic is nil")
	}

	var span trace.Span
	ctx, span = tracer.Start(ctx, fmt.Sprintf("Publish %s", topic))
	span.SetAttributes(attribute.String("topic", topic))
	defer span.End()

	// if the publisher is in closing state or has been closed
	// we return an error and annotate the trace with the error.
	if p.isClose() {
		err := errors.New("publisher is closed")
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())

		return err
	}

	// Prepare attributes that will be passed to the pubsub
	attributes := make(map[string]string)
	attributes["topic"] = topic
	tracingAttributes(span, attributes)

	// Get the topic
	t, err := p.topic(ctx, topic)
	if err != nil {
		return err
	}

	// Setup a timeout for the publisher to give up and attempt to publish the message to the pubsub.
	timeoutCtx, fn := context.WithTimeout(context.Background(), 5*time.Second)
	defer fn()
	_, err = t.Publish(ctx, &gcppubsub.Message{
		Data:       msg,
		Attributes: attributes,
	}).Get(timeoutCtx)

	// in case of error we set the trace to error and return.
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return err
	}

	return nil
}

func (p *Publisher) isClose() bool {
	p.topicsLock.RLock()
	defer p.topicsLock.RUnlock()
	return p.closed
}

func (p *Publisher) topic(ctx context.Context, topic string) (*gcppubsub.Topic, error) {
	p.topicsLock.RLock()
	t, ok := p.topics[topic]
	p.topicsLock.RUnlock()
	if ok {
		return t, nil
	}
	var err error

	p.topicsLock.Lock()
	defer p.topicsLock.Unlock()

	t = p.client.Topic(topic)
	exists, err := t.Exists(ctx)
	if err != nil {
		return nil, errors.Wrapf(err, "could not check if topic %s exists", topic)
	}

	if !exists {
		return nil, errors.Wrap(errors.New("topic does not exist"), topic)
	}

	p.topics[topic] = t
	return t, nil
}
