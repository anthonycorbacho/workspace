package mqtt

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/anthonycorbacho/workspace/kit/pubsub"
	"github.com/eclipse/paho.golang/paho"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
)

var _ pubsub.Publisher = (*Publisher)(nil)

// Publisher publishes a message on a MQTT topic.
type Publisher struct {
	closed    bool
	closeLock sync.RWMutex
	client    *Client
}

// NewPublisher create a new MQTT publisher.
//
// It required a call to Close in order to stop processing messages.
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

	ctx, fn := context.WithTimeout(context.Background(), 10*time.Second)
	defer fn()
	return p.client.Disconnect(ctx)
}

// Publish publishes a message on a MQTT topic.
// It blocks until the message is successfully published or an error occurred.
func (p *Publisher) Publish(ctx context.Context, topic string, msg pubsub.Message, opts ...pubsub.PublishOption) error {
	if len(topic) == 0 {
		return pubsub.ErrNilTopic
	}

	options := &pubsub.PublishOptions{
		Attributes: map[string]string{},
		QoS:        1,
		// by default, we are not setting all message to be retained as they will be kept forever.
		// it will be caller decision to use WithRetain() when publishing a message.
		Retained: false,
	}
	for _, o := range opts {
		o(options)
	}

	ctx, span := tracer.Start(ctx, fmt.Sprintf("Publish %s", topic))
	span.SetAttributes(attribute.String("topic", topic))
	defer span.End()

	// if the publisher is in closing state or has been closed
	// we return an error and annotate the trace with the error.
	if p.isClosed() {
		err := pubsub.ErrPublisherClosed
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return err
	}

	// Prepare attributes that will be passed to the pubsub
	attributes := paho.UserProperties{}
	attributes.Add("topic", topic)
	tracingAttributes(span, attributes)
	// add custom user attributes
	for k, v := range options.Attributes {
		attributes.Add(k, v)
	}

	// Setup a timeout for the publisher to give up and attempt to publish the message to the pubsub.
	timeoutCtx, fn := context.WithTimeout(ctx, 5*time.Second)
	defer fn()

	publish := paho.Publish{
		QoS:    byte(options.QoS),
		Retain: options.Retained,
		Topic:  topic,
		Properties: &paho.PublishProperties{
			User: attributes,
		},
		Payload: msg,
	}

	_, err := p.client.Publish(timeoutCtx, &publish)
	// in case of error we set the trace to error and return.
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return err
	}

	return nil
}

func (p *Publisher) isClosed() bool {
	p.closeLock.RLock()
	defer p.closeLock.RUnlock()

	return p.closed
}
