package kafka

import (
	"context"
	"fmt"
	"os"
	"slices"
	"sync"
	"testing"
	"time"

	kitConfig "github.com/anthonycorbacho/workspace/kit/config"
	"github.com/anthonycorbacho/workspace/kit/id"
	"github.com/anthonycorbacho/workspace/kit/pubsub"
	"github.com/anthonycorbacho/workspace/kit/pubsub/internal/tracingtest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
	"go.opentelemetry.io/otel/trace"
)

type kafkaTestSuite struct {
	suite.Suite
	testURL string
}

func TestKafkaTestSuite(t *testing.T) {
	suite.Run(t, new(kafkaTestSuite))
}

func (suite *kafkaTestSuite) SetupSuite() {
	if os.Getenv("TESTINGKAFKA_URL") == "" {
		suite.T().Skip("No testing kafka setup via env variable TESTINGKAFKA_URL")
	}
	suite.testURL = kitConfig.LookupEnv("TESTINGKAFKA_URL", "127.0.0.1:19092")
	// the trace context is propagated with the global propagator, set by kit/telemetry.
	tracingtest.UsePropagator(suite.T())
}

func (suite *kafkaTestSuite) TestClient() {
	clientID := fmt.Sprintf("test-client-%s", id.New())
	c, err := NewClient([]string{suite.testURL}, WithClientID(clientID))
	suite.NoError(err)
	defer c.Close()
	suite.NoError(c.Ping(context.Background()))
}

func (suite *kafkaTestSuite) TestPublishAndSubscribe() {
	clientID := fmt.Sprintf("test-client-%s", id.New())
	c, err := NewClient([]string{suite.testURL}, WithClientID(clientID), WithAutoTopicCreation())
	suite.NoError(err)
	defer c.Close()

	publisher, err := NewPublisher(c)
	suite.NoError(err)
	defer publisher.Close()

	// Custom sub

	subscriber, err := NewSubscriber(c)
	suite.NoError(err)
	defer subscriber.Close()

	choke := make(chan string, 1)
	fn := func(ctx context.Context, msg pubsub.Message, meta map[string]string) error {
		choke <- string(msg)
		return nil
	}

	err = subscriber.Subscribe(context.Background(), "my-topic-1", fn)
	suite.NoError(err)
	defer subscriber.Close()

	err = publisher.Publish(
		context.Background(),
		"my-topic-1",
		[]byte("a message"),
		pubsub.WithKey("company_id_1"),
		pubsub.WithAttributes(map[string]string{"attb1": "value"}),
	)
	suite.NoError(err)

	select {
	case result := <-choke:
		assert.Equal(suite.T(), "a message", result)
	case <-time.After(time.Second * 10):
		assert.Fail(suite.T(), "timeout")

	}
}

func (suite *kafkaTestSuite) TestConcurrentClose() {
	c, err := NewClient([]string{suite.testURL}, WithClientID(fmt.Sprintf("test-client-%s", id.New())))
	suite.Require().NoError(err)
	subscriber, err := NewSubscriber(c)
	suite.Require().NoError(err)

	var wg sync.WaitGroup
	for range 10 {
		wg.Go(func() { suite.NoError(subscriber.Close()) })
	}
	wg.Wait()
	suite.ErrorIs(subscriber.Subscribe(context.Background(), "my-topic", func(context.Context, pubsub.Message, map[string]string) error {
		return nil
	}), pubsub.ErrSubscriberClosed)
}

// kafkaTimeout is generous as joining a consumer group can take a few seconds.
const kafkaTimeout = 20 * time.Second

func (suite *kafkaTestSuite) newClient(opts ...Option) *Client {
	opts = append([]Option{WithClientID(fmt.Sprintf("test-client-%s", id.New()))}, opts...)
	c, err := NewClient([]string{suite.testURL}, opts...)
	suite.Require().NoError(err)
	return c
}

// publish publishes the messages with a dedicated client, creating the topic if needed.
func (suite *kafkaTestSuite) publish(topic string, msgs ...string) {
	p, err := NewPublisher(suite.newClient(WithAutoTopicCreation()))
	suite.Require().NoError(err)
	defer p.Close()
	for _, m := range msgs {
		suite.Require().NoError(p.Publish(context.Background(), topic, []byte(m)))
	}
}

func (suite *kafkaTestSuite) TestPublishAndSubscribeMetadata() {
	topic := fmt.Sprintf("kit-meta-%s", id.New())

	type received struct {
		msg      string
		metadata map[string]string
		topic    string
		traceID  trace.TraceID
	}
	ch := make(chan received, 1)
	s, err := NewSubscriber(suite.newClient(WithConsumerGroup("group-"+id.New()), WithAutoCommit()))
	suite.Require().NoError(err)
	defer s.Close()

	p, err := NewPublisher(suite.newClient(WithAutoTopicCreation()))
	suite.Require().NoError(err)
	defer p.Close()

	sc := trace.NewSpanContext(trace.SpanContextConfig{
		TraceID:    trace.TraceID{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16},
		SpanID:     trace.SpanID{1, 2, 3, 4, 5, 6, 7, 8},
		TraceFlags: trace.FlagsSampled,
	})
	ctx := trace.ContextWithSpanContext(context.Background(), sc)
	err = p.Publish(ctx, topic, []byte("hello"), pubsub.WithKey("key-1"), pubsub.WithAttributes(map[string]string{"attb1": "value"}))
	suite.Require().NoError(err)

	err = s.Subscribe(context.Background(), topic, func(ctx context.Context, msg pubsub.Message, metadata map[string]string) error {
		ch <- received{msg: msg.String(), metadata: metadata, topic: pubsub.Topic(ctx), traceID: trace.SpanContextFromContext(ctx).TraceID()}
		return nil
	})
	suite.Require().NoError(err)

	select {
	case r := <-ch:
		suite.Equal("hello", r.msg)
		suite.Equal(topic, r.topic)
		suite.Equal(topic, r.metadata["topic"])
		suite.Equal("value", r.metadata["attb1"])
		suite.Contains(r.metadata["traceparent"], sc.TraceID().String())
		suite.Contains(r.metadata, "partition")
		suite.Contains(r.metadata, "offset")
		suite.Equal(sc.TraceID(), r.traceID)
	case <-time.After(kafkaTimeout):
		suite.FailNow("timeout waiting for message")
	}
}

// TestSubscribeAcksOnSuccess checks that Subscribe acks a record once handled and nacks it when
// the handler failed, so it is consumed again before the next records of the partition.
func (suite *kafkaTestSuite) TestSubscribeAcksOnSuccess() {
	tests := []struct {
		name      string
		failFirst bool
		want      []string
	}{
		{name: "handled record is acked", want: []string{"first", "second"}},
		{name: "failed record is consumed again", failFirst: true, want: []string{"first", "first", "second", "second"}},
	}
	for _, tt := range tests {
		suite.Run(tt.name, func() {
			topic := fmt.Sprintf("kit-ack-%s", id.New())
			suite.publish(topic, "first", "second")

			s, err := NewSubscriber(suite.newClient(WithConsumerGroup("group-" + id.New())))
			suite.Require().NoError(err)
			defer s.Close()

			var mu sync.Mutex
			deliveries := []string{}
			handled := make(chan string, 10)
			err = s.Subscribe(context.Background(), topic, func(_ context.Context, msg pubsub.Message, _ map[string]string) error {
				mu.Lock()
				deliveries = append(deliveries, msg.String())
				first := !slices.Contains(deliveries[:len(deliveries)-1], msg.String())
				mu.Unlock()
				if tt.failFirst && first {
					return fmt.Errorf("handler failed for %s", msg)
				}
				select {
				case handled <- msg.String():
				default:
				}
				return nil
			})
			suite.Require().NoError(err)

			for _, want := range []string{"first", "second"} {
				select {
				case m := <-handled:
					suite.Equal(want, m)
				case <-time.After(kafkaTimeout):
					suite.FailNow("timeout waiting for messages")
				}
			}

			// a record that is not acked would be consumed again right away.
			time.Sleep(time.Second)
			mu.Lock()
			defer mu.Unlock()
			suite.Equal(tt.want, deliveries)
		})
	}
}

// TestManualCommitRedelivery checks that a record that is not acked is redelivered to the next
// consumer of the group, while an acked one is not.
func (suite *kafkaTestSuite) TestManualCommitRedelivery() {
	tests := []struct {
		name       string
		autoCommit bool
		ack        bool
		wantFirst  string
	}{
		{name: "ack commits the offset", ack: true, wantFirst: "m3"},
		{name: "nack keeps the offset", ack: false, wantFirst: "m1"},
		{name: "auto commit: ack commits the offset", autoCommit: true, ack: true, wantFirst: "m3"},
		{name: "auto commit: nack keeps the offset", autoCommit: true, ack: false, wantFirst: "m1"},
	}
	for _, tt := range tests {
		suite.Run(tt.name, func() {
			topic := fmt.Sprintf("kit-commit-%s", id.New())
			group := fmt.Sprintf("group-%s", id.New())
			suite.publish(topic, "m1", "m2")

			opts := []Option{WithConsumerGroup(group)}
			if tt.autoCommit {
				opts = append(opts, WithAutoCommit())
			}

			// first consumer handles m1 (and m2 once m1 is acked) then leaves the group.
			s1, err := NewSubscriber(suite.newClient(opts...))
			suite.Require().NoError(err)
			handled := make(chan string, 10)
			err = s1.SubscribeWithAck(context.Background(), topic, func(_ context.Context, msg pubsub.Message, _ map[string]string, ack func(), nack func()) error {
				// m2 is acked so committing it would commit past a nacked m1.
				if tt.ack || msg.String() != "m1" {
					ack()
				} else {
					nack()
				}
				select {
				case handled <- msg.String():
				default:
				}
				return nil
			})
			suite.Require().NoError(err)
			want := []string{"m1", "m2"}
			if !tt.ack {
				// m1 is consumed again until acked, m2 following it is not handled.
				want = []string{"m1", "m1"}
			}
			for _, w := range want {
				select {
				case m := <-handled:
					suite.Equal(w, m)
				case <-time.After(kafkaTimeout):
					suite.FailNow("timeout waiting for first delivery")
				}
			}
			suite.Require().NoError(s1.Close())

			suite.publish(topic, "m3")

			// second consumer of the same group resumes from the committed offset.
			s2, err := NewSubscriber(suite.newClient(WithConsumerGroup(group)))
			suite.Require().NoError(err)
			defer s2.Close()
			first := make(chan string, 1)
			err = s2.SubscribeWithAck(context.Background(), topic, func(_ context.Context, msg pubsub.Message, _ map[string]string, ack func(), _ func()) error {
				ack()
				select {
				case first <- msg.String():
				default:
				}
				return nil
			})
			suite.Require().NoError(err)
			select {
			case m := <-first:
				suite.Equal(tt.wantFirst, m)
			case <-time.After(kafkaTimeout):
				suite.FailNow("timeout waiting for second delivery")
			}
		})
	}
}

func (suite *kafkaTestSuite) TestRegexSubscription() {
	prefix := fmt.Sprintf("kit-regex-%s", id.New())
	ordersTopic := prefix + "-orders"
	usersTopic := prefix + "-users"
	// topics must exist before the regex consumer fetches the metadata.
	suite.publish(ordersTopic, "order")
	suite.publish(usersTopic, "user")

	ordersPattern := "^" + prefix + "-orders.*"
	usersPattern := "^" + prefix + "-users.*"
	s, err := NewSubscriber(suite.newClient(WithRegex([]string{ordersPattern, usersPattern})))
	suite.Require().NoError(err)
	defer s.Close()

	type received struct {
		pattern string
		topic   string
		msg     string
	}
	ch := make(chan received, 2)
	handlerFor := func(pattern string) pubsub.Handler {
		return func(ctx context.Context, msg pubsub.Message, _ map[string]string) error {
			ch <- received{pattern: pattern, topic: pubsub.Topic(ctx), msg: msg.String()}
			return nil
		}
	}
	suite.Require().NoError(s.Subscribe(context.Background(), ordersPattern, handlerFor(ordersPattern)))
	suite.Require().NoError(s.Subscribe(context.Background(), usersPattern, handlerFor(usersPattern)))
	suite.Error(s.Subscribe(context.Background(), "^not-configured.*", handlerFor("")))

	got := map[string]received{}
	for range 2 {
		select {
		case r := <-ch:
			got[r.msg] = r
		case <-time.After(kafkaTimeout):
			suite.FailNow("timeout waiting for messages")
		}
	}
	suite.Equal(received{pattern: ordersPattern, topic: ordersTopic, msg: "order"}, got["order"])
	suite.Equal(received{pattern: usersPattern, topic: usersTopic, msg: "user"}, got["user"])
}

func (suite *kafkaTestSuite) TestPublishCanceledContext() {
	p, err := NewPublisher(suite.newClient(WithAutoTopicCreation()))
	suite.Require().NoError(err)
	defer p.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	suite.Error(p.Publish(ctx, fmt.Sprintf("kit-cancel-%s", id.New()), []byte("x")))
}
