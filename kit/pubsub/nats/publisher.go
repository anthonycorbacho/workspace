package nats

import (
	"context"
	"time"

	"github.com/anthonycorbacho/workspace/kit/errors"
	"github.com/anthonycorbacho/workspace/kit/pubsub"
	"github.com/anthonycorbacho/workspace/kit/pubsub/internal/tracing"
	nats "github.com/nats-io/nats.go"
)

var _ pubsub.Publisher = (*Publisher)(nil)

// publishTimeout is the maximum duration of a publish, unless the context is done before.
const publishTimeout = 5 * time.Second

// Publisher publishes a message on a NATS JetStream Stream's Pub/Sub topic.
//
// Subjects (topics) are managed by the server automatically following presence/absence of subscriptions
// https://docs.nats.io/reference/faq#how-do-i-create-subjects
//
// For more info on how NATS JetStream work, check https://docs.nats.io/using-nats/developer/develop_jetstream.
type Publisher struct {
	nc *nats.Conn
	js nats.JetStreamContext
}

// NewPublisher creates a NATS JetStream publisher using the connection nc and its JetStream context js.
//
// Close must be called to publish the pending messages and close the connection.
func NewPublisher(nc *nats.Conn, js nats.JetStreamContext) (*Publisher, error) {
	if nc == nil {
		return nil, errors.New("invalid nats connection")
	}
	if js == nil {
		return nil, errors.New("invalid jet stream connection")
	}

	return &Publisher{
		nc: nc,
		js: js,
	}, nil
}

// Close drains the connection: the pending messages are published before the connection is closed.
func (p *Publisher) Close() error {
	if p.isClosed() {
		return pubsub.ErrPublisherClosed
	}
	return p.nc.Drain()
}

// isClosed reports if the connection is closed or being closed,
// Close drains the connection asynchronously.
func (p *Publisher) isClosed() bool {
	return p.nc.IsClosed() || p.nc.IsDraining()
}

// Publish publishes a message on a NATS Pub/Sub subject (topic).
//
// It will be received by subscriber(s) in all cases,
// however to enable persistence of the message a Stream must be created
// JetStream publish calls are acknowledged by the JetStream enabled servers
// To receive messages published to a topic, you must create a subscription to that topic.
//
// See https://docs.nats.io/nats-concepts/jetstream/streams to find out more about how NATS streams work.
func (p *Publisher) Publish(ctx context.Context, topic string, msg pubsub.Message, opts ...pubsub.PublishOption) error {
	if len(topic) == 0 {
		return pubsub.ErrNilTopic
	}

	options := pubsub.PublishOptions{}
	for _, o := range opts {
		o(&options)
	}

	// the subject header is set by the publisher and the trace context is injected in the
	// headers, they can't be overridden by the attributes of the options.
	headers := make(nats.Header, len(options.Attributes)+2)
	for k, v := range options.Attributes {
		headers.Set(k, v)
	}
	headers.Set(subjectHeader, topic)
	natsMsg := &nats.Msg{
		Subject: topic,
		Header:  headers,
		Data:    msg,
	}
	ctx, span := startPublishSpan(ctx, tracer, natsMsg)
	defer span.End()

	// if the publisher is in closing state or has been closed
	// we return an error and annotate the trace with the error.
	if p.isClosed() {
		err := pubsub.ErrPublisherClosed
		tracing.RecordError(span, err)
		return err
	}

	timeoutCtx, cancel := context.WithTimeout(ctx, publishTimeout)
	defer cancel()
	_, err := p.js.PublishMsg(natsMsg, nats.Context(timeoutCtx))

	// in case of error we set the trace to error and return.
	if err != nil {
		tracing.RecordError(span, err)
		return err
	}

	return nil
}
