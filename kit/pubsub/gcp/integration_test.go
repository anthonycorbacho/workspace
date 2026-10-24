package gcp

import (
	"context"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	gcppubsub "cloud.google.com/go/pubsub/v2"
	"cloud.google.com/go/pubsub/v2/apiv1/pubsubpb"
	"github.com/anthonycorbacho/workspace/kit/id"
	"github.com/anthonycorbacho/workspace/kit/pubsub"
	"github.com/anthonycorbacho/workspace/kit/pubsub/internal/tracingtest"
	"github.com/stretchr/testify/suite"
	"go.opentelemetry.io/otel/trace"
)

const testProject = "fake"

type gcpTestSuite struct {
	suite.Suite
	admin *gcppubsub.Client
}

func TestGCPTestSuite(t *testing.T) {
	suite.Run(t, new(gcpTestSuite))
}

func (g *gcpTestSuite) SetupSuite() {
	if os.Getenv("PUBSUB_EMULATOR_HOST") == "" {
		g.T().Skip("Skipping, no testing gcp emulator setup via env variable PUBSUB_EMULATOR_HOST")
	}
	c, err := gcppubsub.NewClient(context.Background(), testProject)
	g.Require().NoError(err)
	g.admin = c
	// the trace context is propagated with the global propagator, set by kit/telemetry.
	tracingtest.UsePropagator(g.T())
}

func (g *gcpTestSuite) TearDownSuite() {
	if g.admin != nil {
		_ = g.admin.Close()
	}
}

// createTopic creates a topic and a subscription attached to it.
func (g *gcpTestSuite) createTopic(ordering bool) (string, string) {
	ctx := context.Background()
	topic := fmt.Sprintf("topic-%s", id.New())
	sub := fmt.Sprintf("sub-%s", id.New())
	_, err := g.admin.TopicAdminClient.CreateTopic(ctx, &pubsubpb.Topic{
		Name: resourceName(testProject, "topics", topic),
	})
	g.Require().NoError(err)
	_, err = g.admin.SubscriptionAdminClient.CreateSubscription(ctx, &pubsubpb.Subscription{
		Name:                  resourceName(testProject, "subscriptions", sub),
		Topic:                 resourceName(testProject, "topics", topic),
		EnableMessageOrdering: ordering,
	})
	g.Require().NoError(err)
	return topic, sub
}

func (g *gcpTestSuite) newClient() *gcppubsub.Client {
	c, err := gcppubsub.NewClient(context.Background(), testProject)
	g.Require().NoError(err)
	return c
}

func (g *gcpTestSuite) TestPublishAndSubscribe() {
	ctx := context.Background()
	topic, sub := g.createTopic(true)

	p, err := NewPublisher(g.newClient())
	g.Require().NoError(err)
	defer p.Close()

	s, err := NewSubscriber(g.newClient())
	g.Require().NoError(err)
	defer s.Close()

	type received struct {
		msg      string
		metadata map[string]string
		topic    string
	}
	choke := make(chan received, 2)
	err = s.Subscribe(ctx, sub, func(ctx context.Context, msg pubsub.Message, metadata map[string]string) error {
		choke <- received{msg: msg.String(), metadata: metadata, topic: pubsub.Topic(ctx)}
		return nil
	})
	g.Require().NoError(err)

	err = p.Publish(ctx, topic, []byte("hello"), pubsub.WithAttributes(map[string]string{"attb1": "value"}))
	g.Require().NoError(err)
	err = p.Publish(ctx, topic, []byte("ordered"), pubsub.WithKey("key-1"))
	g.Require().NoError(err)

	got := map[string]received{}
	for range 2 {
		select {
		case r := <-choke:
			got[r.msg] = r
		case <-time.After(20 * time.Second):
			g.FailNow("timeout waiting for messages")
		}
	}
	g.Equal("value", got["hello"].metadata["attb1"])
	g.Equal(topic, got["hello"].topic)
	g.Contains(got, "ordered")
}

func (g *gcpTestSuite) TestUnknownTopicAndSubscription() {
	ctx := context.Background()

	p, err := NewPublisher(g.newClient())
	g.Require().NoError(err)
	defer p.Close()
	g.Error(p.Publish(ctx, "does-not-exist", []byte("hello")))

	s, err := NewSubscriber(g.newClient())
	g.Require().NoError(err)
	defer s.Close()
	g.Error(s.Subscribe(ctx, "does-not-exist", func(context.Context, pubsub.Message, map[string]string) error {
		return nil
	}))
}

func (g *gcpTestSuite) TestClosedStates() {
	ctx := context.Background()
	topic, sub := g.createTopic(false)

	p, err := NewPublisher(g.newClient())
	g.Require().NoError(err)
	g.NoError(p.Close())
	g.ErrorIs(p.Publish(ctx, topic, []byte("hello")), pubsub.ErrPublisherClosed)

	s, err := NewSubscriber(g.newClient())
	g.Require().NoError(err)
	g.NoError(s.Close())
	g.ErrorIs(s.Subscribe(ctx, sub, func(context.Context, pubsub.Message, map[string]string) error {
		return nil
	}), pubsub.ErrSubscriberClosed)
}

func (g *gcpTestSuite) TestConcurrentClose() {
	ctx := context.Background()
	_, sub := g.createTopic(false)

	s, err := NewSubscriber(g.newClient())
	g.Require().NoError(err)
	g.Require().NoError(s.Subscribe(ctx, sub, func(context.Context, pubsub.Message, map[string]string) error {
		return nil
	}))

	var wg sync.WaitGroup
	for range 10 {
		wg.Go(func() { g.NoError(s.Close()) })
		wg.Go(func() {
			// subscribing while closing either succeeds before the close or is rejected.
			err := s.Subscribe(ctx, sub, func(context.Context, pubsub.Message, map[string]string) error {
				return nil
			})
			if err != nil {
				g.ErrorIs(err, pubsub.ErrSubscriberClosed)
			}
		})
	}
	wg.Wait()
}

func (g *gcpTestSuite) TestTracePropagation() {
	ctx := context.Background()
	topic, sub := g.createTopic(false)

	p, err := NewPublisher(g.newClient())
	g.Require().NoError(err)
	defer p.Close()
	s, err := NewSubscriber(g.newClient())
	g.Require().NoError(err)
	defer s.Close()

	type received struct {
		metadata map[string]string
		traceID  trace.TraceID
	}
	ch := make(chan received, 1)
	err = s.Subscribe(ctx, sub, func(ctx context.Context, _ pubsub.Message, metadata map[string]string) error {
		ch <- received{metadata: metadata, traceID: trace.SpanContextFromContext(ctx).TraceID()}
		return nil
	})
	g.Require().NoError(err)

	sc := trace.NewSpanContext(trace.SpanContextConfig{
		TraceID:    trace.TraceID{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16},
		SpanID:     trace.SpanID{1, 2, 3, 4, 5, 6, 7, 8},
		TraceFlags: trace.FlagsSampled,
	})
	// the topic attribute is reserved by the publisher and cannot be overridden.
	err = p.Publish(trace.ContextWithSpanContext(ctx, sc), topic, []byte("hello"), pubsub.WithAttributes(map[string]string{"topic": "other"}))
	g.Require().NoError(err)

	select {
	case r := <-ch:
		g.Equal(topic, r.metadata["topic"])
		g.Contains(r.metadata["traceparent"], sc.TraceID().String())
		g.Equal(sc.TraceID(), r.traceID)
	case <-time.After(10 * time.Second):
		g.FailNow("timeout waiting for message")
	}
}

func (g *gcpTestSuite) TestSubscribeWithAckNackRedelivery() {
	ctx := context.Background()
	topic, sub := g.createTopic(false)

	p, err := NewPublisher(g.newClient())
	g.Require().NoError(err)
	defer p.Close()
	s, err := NewSubscriber(g.newClient(), WithMaxOutstandingMessages(1))
	g.Require().NoError(err)
	defer s.Close()

	var mu sync.Mutex
	deliveries := 0
	ch := make(chan string, 2)
	err = s.SubscribeWithAck(ctx, sub, func(_ context.Context, msg pubsub.Message, _ map[string]string, ack func(), nack func()) error {
		mu.Lock()
		deliveries++
		first := deliveries == 1
		mu.Unlock()
		if first {
			nack()
		} else {
			ack()
		}
		select {
		case ch <- msg.String():
		default:
		}
		return nil
	})
	g.Require().NoError(err)
	g.Require().NoError(p.Publish(ctx, topic, []byte("redeliver me")))

	for i := range 2 {
		select {
		case msg := <-ch:
			g.Equal("redeliver me", msg, "delivery %d", i)
		case <-time.After(10 * time.Second):
			g.FailNow("timeout waiting for redelivery")
		}
	}
}

// TestSubscribeAcksOnSuccess checks that Subscribe acks a message once handled and nacks it when
// the handler failed, so it is redelivered.
func (g *gcpTestSuite) TestSubscribeAcksOnSuccess() {
	tests := []struct {
		name           string
		failFirst      bool
		wantDeliveries int
	}{
		{name: "handled message is acked", wantDeliveries: 1},
		{name: "failed message is redelivered", failFirst: true, wantDeliveries: 2},
	}
	for _, tt := range tests {
		g.Run(tt.name, func() {
			ctx := context.Background()
			topic, sub := g.createTopic(false)

			p, err := NewPublisher(g.newClient())
			g.Require().NoError(err)
			defer p.Close()
			s, err := NewSubscriber(g.newClient())
			g.Require().NoError(err)
			defer s.Close()

			var mu sync.Mutex
			deliveries := map[string]int{}
			handled := make(chan string, 10)
			err = s.Subscribe(ctx, sub, func(_ context.Context, msg pubsub.Message, _ map[string]string) error {
				mu.Lock()
				deliveries[msg.String()]++
				n := deliveries[msg.String()]
				mu.Unlock()
				if tt.failFirst && n == 1 {
					return fmt.Errorf("handler failed for %s", msg)
				}
				select {
				case handled <- msg.String():
				default:
				}
				return nil
			})
			g.Require().NoError(err)

			g.Require().NoError(p.Publish(ctx, topic, []byte("first")))
			g.Require().NoError(p.Publish(ctx, topic, []byte("second")))

			got := []string{}
			for range 2 {
				select {
				case msg := <-handled:
					got = append(got, msg)
				case <-time.After(20 * time.Second):
					g.FailNow("timeout waiting for messages")
				}
			}
			g.ElementsMatch([]string{"first", "second"}, got)

			// a message that is not acked would be redelivered right away.
			time.Sleep(time.Second)
			mu.Lock()
			defer mu.Unlock()
			g.Equal(map[string]int{"first": tt.wantDeliveries, "second": tt.wantDeliveries}, deliveries)
		})
	}
}

func (g *gcpTestSuite) TestSubscriptionContextCanceled() {
	_, sub := g.createTopic(false)
	s, err := NewSubscriber(g.newClient())
	g.Require().NoError(err)
	noop := func(context.Context, pubsub.Message, map[string]string) error { return nil }

	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	g.Error(s.Subscribe(canceled, sub, noop), "subscription lookup fails with a canceled context")

	ctx, cancel := context.WithCancel(context.Background())
	g.Require().NoError(s.Subscribe(ctx, sub, noop))
	// canceling the context stops receiving, Close must not wait forever.
	cancel()
	done := make(chan error, 1)
	go func() { done <- s.Close() }()
	select {
	case err := <-done:
		g.NoError(err)
	case <-time.After(10 * time.Second):
		g.FailNow("timeout waiting for Close")
	}
}

func (g *gcpTestSuite) TestPublishFullyQualifiedTopic() {
	ctx := context.Background()
	topic, sub := g.createTopic(false)

	p, err := NewPublisher(g.newClient())
	g.Require().NoError(err)
	defer p.Close()
	s, err := NewSubscriber(g.newClient())
	g.Require().NoError(err)
	defer s.Close()

	ch := make(chan string, 1)
	err = s.Subscribe(ctx, resourceName(testProject, "subscriptions", sub), func(_ context.Context, msg pubsub.Message, _ map[string]string) error {
		ch <- msg.String()
		return nil
	})
	g.Require().NoError(err)
	g.Require().NoError(p.Publish(ctx, resourceName(testProject, "topics", topic), []byte("qualified")))

	select {
	case msg := <-ch:
		g.Equal("qualified", msg)
	case <-time.After(10 * time.Second):
		g.FailNow("timeout waiting for message")
	}
}

// TestConcurrentLookups checks that concurrent lookups of a topic or a subscription share a
// single client, and that no topic is added once the publisher is closed.
func (g *gcpTestSuite) TestConcurrentLookups() {
	ctx := context.Background()
	topic, sub := g.createTopic(false)

	p, err := NewPublisher(g.newClient())
	g.Require().NoError(err)
	s, err := NewSubscriber(g.newClient())
	g.Require().NoError(err)
	defer s.Close()

	tests := []struct {
		name   string
		lookup func() (any, error)
	}{
		{name: "topic", lookup: func() (any, error) { return p.topic(ctx, topic) }},
		{name: "subscription", lookup: func() (any, error) { return s.subscription(ctx, sub) }},
	}
	for _, tt := range tests {
		g.Run(tt.name, func() {
			const n = 10
			got := make([]any, n)
			var wg sync.WaitGroup
			for i := range n {
				wg.Go(func() {
					v, err := tt.lookup()
					g.NoError(err)
					got[i] = v
				})
			}
			wg.Wait()
			for _, v := range got {
				g.Same(got[0], v)
			}
		})
	}
	g.Len(p.topics, 1)
	g.Len(s.activeSubscriptions, 1)

	// as if Close ran during the check of the topic, the client is still usable.
	p.closeLock.Lock()
	p.closed = true
	p.closeLock.Unlock()
	other, _ := g.createTopic(false)
	_, err = p.topic(ctx, other)
	g.ErrorIs(err, pubsub.ErrPublisherClosed)
	g.Len(p.topics, 1, "a topic added after Close would never be stopped")

	for _, t := range p.topics {
		t.Stop()
	}
	g.NoError(p.client.Close())
}
