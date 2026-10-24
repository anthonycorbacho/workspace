package mqtt

import (
	"context"
	"sync"
	"time"

	"github.com/anthonycorbacho/workspace/kit/pubsub"
	"github.com/anthonycorbacho/workspace/kit/pubsub/internal/tracing"
	"github.com/eclipse/paho.golang/paho"
)

var _ pubsub.Publisher = (*Publisher)(nil)

const (
	// publishTimeout is the maximum duration of a publish, unless the context is done before.
	publishTimeout = 5 * time.Second
	// disconnectTimeout is the maximum duration of the disconnection on Close.
	disconnectTimeout = 10 * time.Second
)

// Publisher publishes a message on a MQTT topic.
type Publisher struct {
	closed    bool
	closeLock sync.RWMutex
	client    *Client
}

// NewPublisher creates a MQTT publisher using client.
//
// Close must be called to disconnect the client.
func NewPublisher(client *Client) (*Publisher, error) {
	if client == nil {
		return nil, pubsub.ErrNilClient
	}

	return &Publisher{
		client: client,
	}, nil
}

// Close stops publishing and disconnects the client.
func (p *Publisher) Close() error {
	p.closeLock.Lock()
	if p.closed {
		p.closeLock.Unlock()
		return nil
	}
	p.closed = true
	p.closeLock.Unlock()

	// Close has no context, the disconnection is bounded by disconnectTimeout.
	ctx, cancel := context.WithTimeout(context.Background(), disconnectTimeout)
	defer cancel()
	return p.client.Disconnect(ctx)
}

// Publish publishes a message on a MQTT topic, with the QoS 1 unless pubsub.WithQoS is given.
// It blocks until the message is successfully published (acknowledged by the broker for the
// QoS 1 and 2) or an error occurred.
//
// The attributes and the trace context are sent as MQTT v5 user properties.
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

	// the topic property is set by the publisher and the trace context is injected in the
	// properties, they can't be overridden by the attributes of the options.
	properties := make(paho.UserProperties, 0, len(options.Attributes)+3)
	for k, v := range options.Attributes {
		properties.Add(k, v)
	}
	publish := paho.Publish{
		QoS:        byte(options.QoS),
		Retain:     options.Retained,
		Topic:      topic,
		Properties: &paho.PublishProperties{User: properties},
		Payload:    msg,
	}
	userPropertiesCarrier{&publish.Properties.User}.Set(topicProperty, topic)
	ctx, span := startPublishSpan(ctx, tracer, &publish)
	defer span.End()

	// if the publisher is in closing state or has been closed
	// we return an error and annotate the trace with the error.
	if p.isClosed() {
		err := pubsub.ErrPublisherClosed
		tracing.RecordError(span, err)
		return err
	}

	// Setup a timeout for the publisher to give up and attempt to publish the message to the pubsub.
	timeoutCtx, cancel := context.WithTimeout(ctx, publishTimeout)
	defer cancel()

	_, err := p.client.Publish(timeoutCtx, &publish)
	// in case of error we set the trace to error and return.
	if err != nil {
		tracing.RecordError(span, err)
		return err
	}

	return nil
}

func (p *Publisher) isClosed() bool {
	p.closeLock.RLock()
	defer p.closeLock.RUnlock()

	return p.closed
}
