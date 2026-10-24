package mqtt

import (
	"context"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	kitConfig "github.com/anthonycorbacho/workspace/kit/config"
	"github.com/anthonycorbacho/workspace/kit/id"
	"github.com/anthonycorbacho/workspace/kit/pubsub"
	"github.com/anthonycorbacho/workspace/kit/pubsub/internal/tracingtest"
	"github.com/eclipse/paho.golang/paho"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
	"go.opentelemetry.io/otel/trace"
)

type mqttTestSuite struct {
	suite.Suite
	testURL string
}

func TestMQTTTestSuite(t *testing.T) {
	suite.Run(t, new(mqttTestSuite))
}

func (m *mqttTestSuite) SetupSuite() {
	if os.Getenv("TESTINGMQTT_URL") == "" {
		m.T().Skip("No testing mqtt setup via env variable TESTINGMQTT_URL")
	}
	m.testURL = kitConfig.LookupEnv("TESTINGMQTT_URL", "mqtt://127.0.0.1:1883")
	// the trace context is propagated with the global propagator, set by kit/telemetry.
	tracingtest.UsePropagator(m.T())
}

func (m *mqttTestSuite) TestClient() {
	clientID := fmt.Sprintf("test-client-%s", id.New())
	c, err := NewClient(context.Background(), m.testURL, clientID)
	m.NoError(err)
	defer c.Disconnect(context.Background())

	// Try to publish to make sure we dont get any error.
	_, err = c.Publish(context.Background(), &paho.Publish{
		Topic:   clientID,
		Payload: []byte("whatever."),
	})

	m.NoError(err)
}

func (m *mqttTestSuite) TestPublishAndSubscribe() {
	clientID := fmt.Sprintf("test-client-%s", id.New())
	c, err := NewClient(context.Background(), m.testURL, clientID)
	m.NoError(err)
	defer c.Disconnect(context.Background())

	publisher, err := NewPublisher(c)
	m.NoError(err)
	defer publisher.Close()

	// Custom sub

	subscriber, err := NewSubscriber(c)
	m.NoError(err)
	defer subscriber.Close()

	choke := make(chan string, 1)
	fn := func(ctx context.Context, msg pubsub.Message, meta map[string]string) error {
		choke <- string(msg)
		return nil
	}

	err = subscriber.Subscribe(context.Background(), clientID, fn)
	m.NoError(err)

	err = publisher.Publish(
		context.Background(),
		clientID,
		[]byte("a message"),
		pubsub.WithQoS(0),
		pubsub.WithAttributes(map[string]string{"attb1": "value"}),
	)
	m.NoError(err)

	select {
	case result := <-choke:
		assert.Equal(m.T(), "a message", result)
	case <-time.After(time.Second * 10):
		assert.Fail(m.T(), "timeout")

	}
}

func (m *mqttTestSuite) TestConcurrentClose() {
	c, err := NewClient(context.Background(), m.testURL, fmt.Sprintf("test-client-%s", id.New()))
	m.Require().NoError(err)
	subscriber, err := NewSubscriber(c)
	m.Require().NoError(err)

	var wg sync.WaitGroup
	for range 10 {
		wg.Go(func() { m.NoError(subscriber.Close()) })
	}
	wg.Wait()
	m.ErrorIs(subscriber.Subscribe(context.Background(), "my-topic", func(context.Context, pubsub.Message, map[string]string) error {
		return nil
	}), pubsub.ErrSubscriberClosed)
}

func (m *mqttTestSuite) newClient() *Client {
	c, err := NewClient(context.Background(), m.testURL, fmt.Sprintf("test-client-%s", id.New()))
	m.Require().NoError(err)
	m.T().Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = c.Disconnect(ctx)
	})
	return c
}

type mqttReceived struct {
	msg      string
	metadata map[string]string
	topic    string
	traceID  trace.TraceID
}

func (m *mqttTestSuite) TestPublishQoS() {
	tests := []struct {
		name string
		opts []pubsub.PublishOption
	}{
		{name: "default qos"},
		{name: "qos 0", opts: []pubsub.PublishOption{pubsub.WithQoS(0)}},
		{name: "qos 1", opts: []pubsub.PublishOption{pubsub.WithQoS(1)}},
		{name: "qos 2", opts: []pubsub.PublishOption{pubsub.WithQoS(2)}},
		{name: "out of range qos", opts: []pubsub.PublishOption{pubsub.WithQoS(7)}},
	}
	for _, tt := range tests {
		m.Run(tt.name, func() {
			topic := fmt.Sprintf("kit/qos/%s", id.New())
			c := m.newClient()
			p, err := NewPublisher(c)
			m.Require().NoError(err)
			s, err := NewSubscriber(c)
			m.Require().NoError(err)

			ch := make(chan mqttReceived, 1)
			err = s.Subscribe(context.Background(), topic, func(ctx context.Context, msg pubsub.Message, metadata map[string]string) error {
				ch <- mqttReceived{msg: msg.String(), metadata: metadata, topic: pubsub.Topic(ctx)}
				return nil
			})
			m.Require().NoError(err)

			opts := append([]pubsub.PublishOption{pubsub.WithAttributes(map[string]string{"attb1": "value"})}, tt.opts...)
			m.Require().NoError(p.Publish(context.Background(), topic, []byte("hello"), opts...))

			select {
			case r := <-ch:
				m.Equal("hello", r.msg)
				m.Equal(topic, r.topic)
				m.Equal(topic, r.metadata["topic"])
				m.Equal("value", r.metadata["attb1"])
			case <-time.After(10 * time.Second):
				m.FailNow("timeout waiting for message")
			}
		})
	}
}

func (m *mqttTestSuite) TestTracePropagation() {
	topic := fmt.Sprintf("kit/trace/%s", id.New())
	c := m.newClient()
	p, err := NewPublisher(c)
	m.Require().NoError(err)
	s, err := NewSubscriber(c)
	m.Require().NoError(err)

	ch := make(chan mqttReceived, 1)
	err = s.Subscribe(context.Background(), topic, func(ctx context.Context, msg pubsub.Message, metadata map[string]string) error {
		ch <- mqttReceived{msg: msg.String(), metadata: metadata, traceID: trace.SpanContextFromContext(ctx).TraceID()}
		return nil
	})
	m.Require().NoError(err)

	sc := trace.NewSpanContext(trace.SpanContextConfig{
		TraceID: trace.TraceID{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16},
		SpanID:  trace.SpanID{1, 2, 3, 4, 5, 6, 7, 8},
	})
	ctx := trace.ContextWithSpanContext(context.Background(), sc)
	m.Require().NoError(p.Publish(ctx, topic, []byte("hello")))

	select {
	case r := <-ch:
		m.Contains(r.metadata["traceparent"], sc.TraceID().String())
		m.Equal(sc.TraceID(), r.traceID)
	case <-time.After(10 * time.Second):
		m.FailNow("timeout waiting for message")
	}
}

func (m *mqttTestSuite) TestWildcardSubscription() {
	prefix := fmt.Sprintf("kit/wildcard/%s", id.New())
	c := m.newClient()
	p, err := NewPublisher(c)
	m.Require().NoError(err)
	s, err := NewSubscriber(c)
	m.Require().NoError(err)

	ch := make(chan mqttReceived, 2)
	err = s.Subscribe(context.Background(), prefix+"/+", func(ctx context.Context, msg pubsub.Message, _ map[string]string) error {
		ch <- mqttReceived{msg: msg.String(), topic: pubsub.Topic(ctx)}
		return nil
	})
	m.Require().NoError(err)

	m.Require().NoError(p.Publish(context.Background(), prefix+"/a", []byte("a")))
	m.Require().NoError(p.Publish(context.Background(), prefix+"/b", []byte("b")))

	got := map[string]string{}
	for range 2 {
		select {
		case r := <-ch:
			got[r.msg] = r.topic
		case <-time.After(10 * time.Second):
			m.FailNow("timeout waiting for messages")
		}
	}
	m.Equal(map[string]string{"a": prefix + "/a", "b": prefix + "/b"}, got)
}

// TestSubscribeRawMessage checks that a message published without user properties
// (i.e. not by this package) gets its topic from the MQTT packet.
func (m *mqttTestSuite) TestSubscribeRawMessage() {
	topic := fmt.Sprintf("kit/raw/%s", id.New())
	c := m.newClient()
	s, err := NewSubscriber(c)
	m.Require().NoError(err)

	ch := make(chan mqttReceived, 1)
	err = s.Subscribe(context.Background(), topic, func(ctx context.Context, msg pubsub.Message, metadata map[string]string) error {
		ch <- mqttReceived{msg: msg.String(), metadata: metadata, topic: pubsub.Topic(ctx)}
		return nil
	})
	m.Require().NoError(err)

	_, err = c.Publish(context.Background(), &paho.Publish{Topic: topic, QoS: 1, Payload: []byte("raw")})
	m.Require().NoError(err)

	select {
	case r := <-ch:
		m.Equal("raw", r.msg)
		m.Equal(topic, r.topic)
		m.Equal(map[string]string{"topic": topic}, r.metadata)
	case <-time.After(10 * time.Second):
		m.FailNow("timeout waiting for message")
	}
}

func (m *mqttTestSuite) TestPublishRetained() {
	topic := fmt.Sprintf("kit/retain/%s", id.New())
	p, err := NewPublisher(m.newClient())
	m.Require().NoError(err)
	m.Require().NoError(p.Publish(context.Background(), topic, []byte("retained"), pubsub.WithRetain()))
	defer func() {
		// an empty retained payload clears the retained message on the broker.
		m.NoError(p.Publish(context.Background(), topic, []byte{}, pubsub.WithRetain()))
	}()

	// a new subscriber asking for retained messages receives it with the retain flag.
	c := m.newClient()
	type retained struct {
		payload string
		retain  bool
	}
	ch := make(chan retained, 1)
	c.route.RegisterHandler(topic, func(p *paho.Publish) {
		ch <- retained{payload: string(p.Payload), retain: p.Retain}
	})
	_, err = c.Subscribe(context.Background(), &paho.Subscribe{
		Subscriptions: []paho.SubscribeOptions{{Topic: topic, QoS: 1}},
	})
	m.Require().NoError(err)

	select {
	case r := <-ch:
		m.Equal(retained{payload: "retained", retain: true}, r)
	case <-time.After(10 * time.Second):
		m.FailNow("timeout waiting for retained message")
	}
}

func (m *mqttTestSuite) TestHandlerErrorKeepsSubscription() {
	topic := fmt.Sprintf("kit/err/%s", id.New())
	c := m.newClient()
	p, err := NewPublisher(c)
	m.Require().NoError(err)
	s, err := NewSubscriber(c)
	m.Require().NoError(err)

	ch := make(chan string, 2)
	err = s.SubscribeWithAck(context.Background(), topic, func(_ context.Context, msg pubsub.Message, _ map[string]string, ack func(), nack func()) error {
		// ack and nack are no-ops in MQTT, calling them must be safe.
		ack()
		nack()
		ch <- msg.String()
		return fmt.Errorf("handler failed for %s", msg)
	})
	m.Require().NoError(err)

	m.Require().NoError(p.Publish(context.Background(), topic, []byte("first")))
	m.Require().NoError(p.Publish(context.Background(), topic, []byte("second")))

	got := []string{}
	for range 2 {
		select {
		case msg := <-ch:
			got = append(got, msg)
		case <-time.After(10 * time.Second):
			m.FailNow("timeout waiting for messages")
		}
	}
	m.ElementsMatch([]string{"first", "second"}, got)
}

// TestSubscribeHandlerError checks that Subscribe does not redeliver a message the handler failed
// to process, MQTT having no negative acknowledgement.
func (m *mqttTestSuite) TestSubscribeHandlerError() {
	tests := []struct {
		name    string
		failing bool
	}{
		{name: "handled message"},
		{name: "failed message is not redelivered", failing: true},
	}
	for _, tt := range tests {
		m.Run(tt.name, func() {
			topic := fmt.Sprintf("kit/suberr/%s", id.New())
			c := m.newClient()
			p, err := NewPublisher(c)
			m.Require().NoError(err)
			s, err := NewSubscriber(c)
			m.Require().NoError(err)

			var mu sync.Mutex
			deliveries := map[string]int{}
			handled := make(chan string, 10)
			err = s.Subscribe(context.Background(), topic, func(_ context.Context, msg pubsub.Message, _ map[string]string) error {
				mu.Lock()
				deliveries[msg.String()]++
				mu.Unlock()
				select {
				case handled <- msg.String():
				default:
				}
				if tt.failing {
					return fmt.Errorf("handler failed for %s", msg)
				}
				return nil
			})
			m.Require().NoError(err)

			m.Require().NoError(p.Publish(context.Background(), topic, []byte("first"), pubsub.WithQoS(1)))
			m.Require().NoError(p.Publish(context.Background(), topic, []byte("second"), pubsub.WithQoS(1)))

			got := []string{}
			for range 2 {
				select {
				case msg := <-handled:
					got = append(got, msg)
				case <-time.After(10 * time.Second):
					m.FailNow("timeout waiting for messages")
				}
			}
			m.ElementsMatch([]string{"first", "second"}, got)

			time.Sleep(time.Second)
			mu.Lock()
			defer mu.Unlock()
			m.Equal(map[string]int{"first": 1, "second": 1}, deliveries)
		})
	}
}

func (m *mqttTestSuite) TestClosedStates() {
	topic := fmt.Sprintf("kit/closed/%s", id.New())
	noop := func(context.Context, pubsub.Message, map[string]string) error { return nil }

	p, err := NewPublisher(m.newClient())
	m.Require().NoError(err)
	m.ErrorIs(p.Publish(context.Background(), "", []byte("x")), pubsub.ErrNilTopic)
	m.NoError(p.Close())
	m.NoError(p.Close(), "closing twice is a no-op")
	m.ErrorIs(p.Publish(context.Background(), topic, []byte("x")), pubsub.ErrPublisherClosed)

	s, err := NewSubscriber(m.newClient())
	m.Require().NoError(err)
	m.ErrorIs(s.Subscribe(context.Background(), "", noop), pubsub.ErrNilTopic)
	m.NoError(s.Close())
	m.ErrorIs(s.Subscribe(context.Background(), topic, noop), pubsub.ErrSubscriberClosed)
}

func (m *mqttTestSuite) TestPublishDisconnectedClient() {
	c := m.newClient()
	p, err := NewPublisher(c)
	m.Require().NoError(err)

	m.Require().NoError(c.Disconnect(context.Background()))
	m.Error(p.Publish(context.Background(), fmt.Sprintf("kit/disconnected/%s", id.New()), []byte("x")))
}

func (m *mqttTestSuite) TestClientWithOptions() {
	c, err := NewClient(context.Background(), m.testURL, fmt.Sprintf("test-client-%s", id.New()),
		WithUser("user"), WithPassword("password"), WithKeepAlive(10), WithRetry(time.Second))
	m.Require().NoError(err)
	m.NoError(c.Disconnect(context.Background()))
}
