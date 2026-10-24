package gcp

import (
	"context"
	"maps"
	"sync"
	"time"

	gcppubsub "cloud.google.com/go/pubsub/v2"
	"cloud.google.com/go/pubsub/v2/apiv1/pubsubpb"
	"github.com/anthonycorbacho/workspace/kit/errors"
	"github.com/anthonycorbacho/workspace/kit/pubsub"
	"github.com/anthonycorbacho/workspace/kit/pubsub/internal/tracing"
	grpccodes "google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

var _ pubsub.Publisher = (*Publisher)(nil)

// publishTimeout is the maximum duration of a publish, unless the context is done before.
const publishTimeout = 5 * time.Second

// Publisher publishes a message on a Google Cloud Pub/Sub topic.
//
// For more info on how Google Cloud Pub/Sub Publisher work, check https://cloud.google.com/pubsub/docs/publisher.
type Publisher struct {
	topics     map[string]*gcppubsub.Publisher
	topicsLock sync.RWMutex
	closed     bool
	closeLock  sync.RWMutex
	client     *gcppubsub.Client
}

// NewPublisher creates a Google Cloud Pub/Sub publisher using client, closed by Close.
//
// Close must be called to stop publishing and release the topics.
func NewPublisher(client *gcppubsub.Client) (*Publisher, error) {
	if client == nil {
		return nil, pubsub.ErrNilClient
	}

	return &Publisher{
		topics: map[string]*gcppubsub.Publisher{},
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
func (p *Publisher) Publish(ctx context.Context, topic string, msg pubsub.Message, opts ...pubsub.PublishOption) error {
	if len(topic) == 0 {
		return pubsub.ErrNilTopic
	}

	options := pubsub.PublishOptions{}
	for _, o := range opts {
		o(&options)
	}

	// the topic attribute is set by the publisher and the trace context is injected in the
	// attributes, they can't be overridden by the attributes of the options.
	attributes := make(map[string]string, len(options.Attributes)+3)
	maps.Copy(attributes, options.Attributes)
	attributes[topicAttribute] = topic
	gcpMsg := &gcppubsub.Message{
		Data:        msg,
		Attributes:  attributes,
		OrderingKey: options.Key,
	}
	ctx, span := startPublishSpan(ctx, tracer, topic, gcpMsg)
	defer span.End()

	// if the publisher is in closing state or has been closed
	// we return an error and annotate the trace with the error.
	if p.isClose() {
		err := pubsub.ErrPublisherClosed
		tracing.RecordError(span, err)
		return err
	}

	// Get the topic
	t, err := p.topic(ctx, topic)
	if err != nil {
		tracing.RecordError(span, err)
		return err
	}

	// Setup a timeout for the publisher to give up and attempt to publish the message to the pubsub.
	timeoutCtx, cancel := context.WithTimeout(ctx, publishTimeout)
	defer cancel()
	_, err = t.Publish(ctx, gcpMsg).Get(timeoutCtx)

	// in case of error we set the trace to error and return.
	if err != nil {
		// a failed publish pauses the ordering key, resume it so next messages can be published.
		if options.Key != "" {
			t.ResumePublish(options.Key)
		}
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

// topic returns the publisher of the topic, it checks the topic exists the first time it is used.
func (p *Publisher) topic(ctx context.Context, topic string) (*gcppubsub.Publisher, error) {
	p.topicsLock.RLock()
	t, ok := p.topics[topic]
	p.topicsLock.RUnlock()
	if ok {
		return t, nil
	}

	// checked without the lock so a slow RPC does not block publishing to the other topics.
	_, err := p.client.TopicAdminClient.GetTopic(ctx, &pubsubpb.GetTopicRequest{
		Topic: resourceName(p.client.Project(), "topics", topic),
	})
	if status.Code(err) == grpccodes.NotFound {
		return nil, errors.Wrap(errors.New("topic does not exist"), topic)
	}
	if err != nil {
		return nil, errors.Wrapf(err, "could not check if topic %s exists", topic)
	}

	p.topicsLock.Lock()
	defer p.topicsLock.Unlock()

	// Close stops the topics under the lock, a topic added after it would never be stopped.
	if p.isClose() {
		return nil, pubsub.ErrPublisherClosed
	}
	// another publish may have added the topic during the check.
	if t, ok := p.topics[topic]; ok {
		return t, nil
	}

	t = p.client.Publisher(topic)
	// ordering is only applied to messages published with an ordering key (see pubsub.WithKey).
	t.EnableMessageOrdering = true
	p.topics[topic] = t
	return t, nil
}
