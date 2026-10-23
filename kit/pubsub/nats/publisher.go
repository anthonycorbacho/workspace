package nats

import (
	"context"
	"fmt"
	"time"

	"github.com/anthonycorbacho/workspace/kit/errors"
	"github.com/anthonycorbacho/workspace/kit/pubsub"
	nats "github.com/nats-io/nats.go"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

var _ pubsub.Publisher = (*Publisher)(nil)

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

// NewPublisher create a new Nats JetStream publisher.
//
// It required a call to Close in order to stop processing messages and close topic connections.
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

// Close notifies the Publisher to stop processing messages, send all the remaining messages and close the connection.
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

	var span trace.Span
	_, span = tracer.Start(ctx, fmt.Sprintf("Publish %s", topic))
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

	// Prepare headers that will be passed to the pubsub
	headers := make(map[string][]string)
	for k, v := range options.Attributes {
		headers[k] = []string{v}
	}
	headers["subject"] = []string{topic}
	tracingAttributes(span, headers)
	natsMsg := &nats.Msg{
		Subject: topic,
		Header:  headers,
		Data:    msg,
	}

	timeoutCtx, fn := context.WithTimeout(context.Background(), 5*time.Second)
	defer fn()
	_, err := p.js.PublishMsg(natsMsg, nats.Context(timeoutCtx))

	// in case of error we set the trace to error and return.
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return err
	}

	return nil
}
