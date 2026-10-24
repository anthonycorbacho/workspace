package nats

import (
	"context"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/anthonycorbacho/workspace/kit/id"
	"github.com/anthonycorbacho/workspace/kit/log"
	"github.com/anthonycorbacho/workspace/kit/pubsub"
	"github.com/anthonycorbacho/workspace/kit/pubsub/internal/tracingtest"
	"github.com/nats-io/nats.go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	"go.opentelemetry.io/otel/trace"
)

const test = "test"
const testDefaultSubject = "test.default"
const testSubjects = "test.>"
const testDeliverySubject = "test_redirect"
const testGroup = "testGroup"

type natsTestSuite struct {
	suite.Suite
	nc       *nats.Conn
	js       nats.JetStreamContext
	consumer *nats.ConsumerInfo
	p        *Publisher
	s        *Subscriber
	l        *log.Logger
	ctx      context.Context
	wg       *sync.WaitGroup
	errCh    chan error
}

func TestNATSTestSuite(t *testing.T) {
	suite.Run(t, new(natsTestSuite))
}

// connect connects to the NATS server at url, the options are applied to the connection.
func connect(url string, options ...func(*nats.Conn)) (nats.JetStreamContext, *nats.Conn, error) {
	nc, err := nats.Connect(url)
	if err != nil {
		return nil, nil, err
	}
	for _, option := range options {
		option(nc)
	}
	js, err := nc.JetStream()
	if err != nil {
		nc.Close()
		return nil, nil, err
	}
	return js, nc, nil
}

func errorHandler(errCh chan error) func(*nats.Conn) {
	return func(nc *nats.Conn) {
		nc.SetErrorHandler(func(_ *nats.Conn, _ *nats.Subscription, err error) {
			errCh <- err
		})
	}
}
func closedHandler(wg *sync.WaitGroup) func(*nats.Conn) {
	return func(nc *nats.Conn) {
		nc.SetClosedHandler(func(_ *nats.Conn) {
			wg.Done()
		})
	}
}

func (n *natsTestSuite) SetupSuite() {
	if os.Getenv("TESTINGNATS_URL") == "" {
		n.T().Skip("Skipping, no testing nats setup via env variable TESTINGNATS_URL")
	}
	ctx := context.Background()
	// the trace context is propagated with the global propagator, set by kit/telemetry.
	tracingtest.UsePropagator(n.T())
	// Initiate a logger with pre-configuration for production and telemetry.
	l, err := log.New()
	if err != nil {
		n.T().Fatalf("logging couldn't be setup: %v", err)
	}
	// Replace the global logger with the Service scoped log.
	log.ReplaceGlobal(l)
	wg := sync.WaitGroup{}
	errCh := make(chan error, 1)
	addr, _ := os.LookupEnv("TESTINGNATS_URL")

	js, nc, err := connect(addr, errorHandler(errCh), closedHandler(&wg))
	if err != nil {
		n.T().Fatalf("setting up nats server failed: %v", err)
	}
	_, err = js.AddStream(&nats.StreamConfig{
		Name:     test,
		Subjects: []string{testSubjects},
		Storage:  nats.FileStorage,
	})
	if err != nil {
		n.T().Fatalf("setting upstream failed: %v", err)
	}
	consumer, err := js.AddConsumer(test, &nats.ConsumerConfig{
		Durable: test,
		// Not setting up a filter will forward all messages from the consumer to the Subscriber
		// For example, it would make it receive messages from `testClosingSubject``
		FilterSubject:  testDefaultSubject,
		AckPolicy:      nats.AckExplicitPolicy,
		DeliverSubject: testDeliverySubject,
		DeliverGroup:   testGroup,
	})
	if err != nil {
		n.T().Fatalf("setting consumer failed: %v", err)
	}
	p, err := NewPublisher(nc, js)
	if err != nil {
		n.T().Fatalf("setting up publisher: %v", err)
	}
	s, err := NewSubscriber(testGroup, nc, js, consumer)
	if err != nil {
		n.T().Fatalf("setting up subscriber: %v", err)
	}
	n.js = js
	n.nc = nc
	n.consumer = consumer
	n.p = p
	n.s = s
	n.l = l
	n.ctx = ctx
	n.wg = &wg
	n.errCh = errCh
	// fmt.Println("consumer :", consumer)
	// for info := range n.js.StreamsInfo(nats.Context(ctx)) {
	// 	fmt.Println("stream name:", info)
	// }
}

func (n *natsTestSuite) TearDownSuite() {
	n.nc.Close()
}

func (n *natsTestSuite) TestPublishAndSubscribe() {
	// Given
	n.wg.Add(1)
	ch := make(chan string, 1)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	// ctx := context.TODO()

	// Setting up the case
	n.s.Subscribe(ctx, testDefaultSubject, func(ctx context.Context, msg pubsub.Message, _ map[string]string) error {
		ch <- fmt.Sprintf("Message received: %v", string(msg))
		// Somehow checkHeaders - access ctx?
		return nil
	})

	err := n.p.Publish(ctx, testDefaultSubject, []byte(test))
	if err != nil {
		assert.Fail(n.T(), fmt.Sprintf("publish %v", err))
	}
	select {
	case result := <-ch:
		assert.Equal(n.T(), "Message received: test", result)
	case <-time.After(time.Second):
		assert.Fail(n.T(), "timeout waiting")
	}
	err = n.s.Close()
	if err != nil {
		n.T().Fatalf("setting upstream failed: %v", err)
	}
	n.wg.Wait()
	select {
	case e := <-n.errCh:
		n.T().Fatalf("nats error channel: %v", e)
	default:
	}
}

func (n *natsTestSuite) TestClosedStates() {
	// Given
	const testClosingSubject = "test.close"
	ch := make(chan string, 1)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	addr, _ := os.LookupEnv("TESTINGNATS_URL")
	// Publisher
	js1, nc1, err := connect(addr)
	if err != nil {
		n.T().Fatalf("setting up nats server failed: %v", err)
	}
	p, err := NewPublisher(nc1, js1)
	if err != nil {
		n.T().Fatalf("setting up publisher: %v", err)
	}
	// Subscriber
	js2, nc2, err := connect(addr)
	if err != nil {
		n.T().Fatalf("setting up nats server failed: %v", err)
	}
	consumer, err := n.js.AddConsumer(test, &nats.ConsumerConfig{
		Durable:        test + "closed",
		AckPolicy:      nats.AckExplicitPolicy,
		DeliverSubject: testDeliverySubject + "closed",
		DeliverGroup:   testGroup + "closed",
	})
	if err != nil {
		n.T().Fatalf("setting up consumer: %v", err)
	}
	s, err := NewSubscriber("testClosedGroup", nc2, js2, consumer)
	if err != nil {
		n.T().Fatalf("setting up subscriber: %v", err)
	}

	// Setting up the case
	s.Subscribe(ctx, testClosingSubject, func(ctx context.Context, msg pubsub.Message, _ map[string]string) error {
		fmt.Println("Message received")
		fmt.Println(msg)
		ch <- fmt.Sprintf("Message received: %v", string(msg))
		return nil
	})
	err = s.Close()
	assert.NoError(n.T(), err)
	err = s.Close()
	assert.Error(n.T(), pubsub.ErrSubscriberClosed, err)

	err = p.Publish(ctx, testClosingSubject, []byte("test closed subscriber"))
	if err != nil {
		assert.Fail(n.T(), "publish %v", err)
	}
	select {
	case result := <-ch:
		assert.Fail(n.T(), fmt.Sprintf("closed subscriber received message: %v", result))
	case <-time.After(time.Second):
	}

	p.Close()
	err = p.Publish(ctx, testClosingSubject, []byte("test closed publisher"))
	assert.Error(n.T(), pubsub.ErrPublisherClosed, err)
}

func (n *natsTestSuite) TestConcurrentClose() {
	addr, _ := os.LookupEnv("TESTINGNATS_URL")
	js, nc, err := connect(addr)
	n.Require().NoError(err)
	consumer, err := n.js.AddConsumer(test, &nats.ConsumerConfig{
		Durable:        test + "concurrentclose",
		AckPolicy:      nats.AckExplicitPolicy,
		DeliverSubject: testDeliverySubject + "concurrentclose",
		DeliverGroup:   testGroup + "concurrentclose",
	})
	n.Require().NoError(err)
	s, err := NewSubscriber(testGroup+"concurrentclose", nc, js, consumer)
	n.Require().NoError(err)

	var wg sync.WaitGroup
	for range 10 {
		wg.Go(func() { _ = s.Close() })
	}
	wg.Wait()
	n.ErrorIs(s.Subscribe(context.Background(), testDefaultSubject, func(context.Context, pubsub.Message, map[string]string) error {
		return nil
	}), pubsub.ErrSubscriberClosed)
}

// natsFixture is an isolated connection, stream and consumer so tests don't share state on the broker.
type natsFixture struct {
	nc       *nats.Conn
	js       nats.JetStreamContext
	subject  string
	consumer *nats.ConsumerInfo
}

// newFixture creates a uniquely named stream with a push consumer bound to a queue group.
func (n *natsTestSuite) newFixture() *natsFixture {
	addr, _ := os.LookupEnv("TESTINGNATS_URL")
	js, nc, err := connect(addr)
	n.Require().NoError(err)

	uid := id.New()
	name := "kit-" + uid
	subject := "kit." + uid
	_, err = js.AddStream(&nats.StreamConfig{Name: name, Subjects: []string{subject + ".>"}})
	n.Require().NoError(err)
	consumer, err := js.AddConsumer(name, &nats.ConsumerConfig{
		Durable:        name,
		AckPolicy:      nats.AckExplicitPolicy,
		AckWait:        30 * time.Second,
		DeliverSubject: "deliver." + uid,
		DeliverGroup:   name,
	})
	n.Require().NoError(err)

	n.T().Cleanup(func() {
		// the subscriber may have drained the connection already.
		_, cleanNC, err := connect(addr)
		if err != nil {
			return
		}
		defer cleanNC.Close()
		cleanJS, err := cleanNC.JetStream()
		if err == nil {
			_ = cleanJS.DeleteStream(name)
		}
		nc.Close()
	})
	return &natsFixture{nc: nc, js: js, subject: subject + ".events", consumer: consumer}
}

func (n *natsTestSuite) TestPublishAndSubscribeMetadata() {
	f := n.newFixture()
	p, err := NewPublisher(f.nc, f.js)
	n.Require().NoError(err)
	s, err := NewSubscriber(f.consumer.Name, f.nc, f.js, f.consumer)
	n.Require().NoError(err)

	type received struct {
		msg      string
		metadata map[string]string
		topic    string
		traceID  trace.TraceID
	}
	ch := make(chan received, 1)
	err = s.Subscribe(context.Background(), f.subject, func(ctx context.Context, msg pubsub.Message, metadata map[string]string) error {
		ch <- received{msg: msg.String(), metadata: metadata, topic: pubsub.Topic(ctx), traceID: trace.SpanContextFromContext(ctx).TraceID()}
		return nil
	})
	n.Require().NoError(err)

	sc := trace.NewSpanContext(trace.SpanContextConfig{
		TraceID:    trace.TraceID{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16},
		SpanID:     trace.SpanID{1, 2, 3, 4, 5, 6, 7, 8},
		TraceFlags: trace.FlagsSampled,
	})
	ctx := trace.ContextWithSpanContext(context.Background(), sc)
	err = p.Publish(ctx, f.subject, []byte("hello"), pubsub.WithAttributes(map[string]string{"attb1": "value"}))
	n.Require().NoError(err)

	select {
	case r := <-ch:
		n.Equal("hello", r.msg)
		n.Equal(f.subject, r.topic)
		n.Equal("value", r.metadata["attb1"])
		n.Equal(f.subject, r.metadata["subject"])
		n.Contains(r.metadata["traceparent"], sc.TraceID().String())
		n.Equal(sc.TraceID(), r.traceID)
	case <-time.After(10 * time.Second):
		n.FailNow("timeout waiting for message")
	}
}

func (n *natsTestSuite) TestSubscribeWithAckNackRedelivery() {
	f := n.newFixture()
	p, err := NewPublisher(f.nc, f.js)
	n.Require().NoError(err)
	s, err := NewSubscriber(f.consumer.Name, f.nc, f.js, f.consumer)
	n.Require().NoError(err)

	var mu sync.Mutex
	deliveries := 0
	ch := make(chan string, 2)
	err = s.SubscribeWithAck(context.Background(), f.subject, func(_ context.Context, msg pubsub.Message, _ map[string]string, ack func(), nack func()) error {
		mu.Lock()
		deliveries++
		first := deliveries == 1
		mu.Unlock()
		if first {
			nack()
		} else {
			ack()
		}
		ch <- msg.String()
		return nil
	})
	n.Require().NoError(err)
	n.Require().NoError(p.Publish(context.Background(), f.subject, []byte("redeliver me")))

	for i := range 2 {
		select {
		case msg := <-ch:
			n.Equal("redeliver me", msg, "delivery %d", i)
		case <-time.After(10 * time.Second):
			n.FailNow("timeout waiting for redelivery")
		}
	}
}

// TestSubscribeAcksOnSuccess checks that Subscribe acks a message once handled and nacks it when
// the handler failed, so it is redelivered.
func (n *natsTestSuite) TestSubscribeAcksOnSuccess() {
	tests := []struct {
		name           string
		failFirst      bool
		wantDeliveries int
	}{
		{name: "handled message is acked", wantDeliveries: 1},
		{name: "failed message is redelivered", failFirst: true, wantDeliveries: 2},
	}
	for _, tt := range tests {
		n.Run(tt.name, func() {
			f := n.newFixture()
			p, err := NewPublisher(f.nc, f.js)
			n.Require().NoError(err)
			s, err := NewSubscriber(f.consumer.Name, f.nc, f.js, f.consumer)
			n.Require().NoError(err)

			var mu sync.Mutex
			deliveries := map[string]int{}
			handled := make(chan string, 10)
			err = s.Subscribe(context.Background(), f.subject, func(_ context.Context, msg pubsub.Message, _ map[string]string) error {
				mu.Lock()
				deliveries[msg.String()]++
				count := deliveries[msg.String()]
				mu.Unlock()
				if tt.failFirst && count == 1 {
					return fmt.Errorf("handler failed for %s", msg)
				}
				select {
				case handled <- msg.String():
				default:
				}
				return nil
			})
			n.Require().NoError(err)

			n.Require().NoError(p.Publish(context.Background(), f.subject, []byte("first")))
			n.Require().NoError(p.Publish(context.Background(), f.subject, []byte("second")))

			got := []string{}
			for range 2 {
				select {
				case msg := <-handled:
					got = append(got, msg)
				case <-time.After(10 * time.Second):
					n.FailNow("timeout waiting for messages")
				}
			}
			n.ElementsMatch([]string{"first", "second"}, got)

			// a message that is not acked would be redelivered right away.
			time.Sleep(time.Second)
			mu.Lock()
			defer mu.Unlock()
			n.Equal(map[string]int{"first": tt.wantDeliveries, "second": tt.wantDeliveries}, deliveries)
		})
	}
}

func (n *natsTestSuite) TestPublishSubscribeErrors() {
	f := n.newFixture()
	p, err := NewPublisher(f.nc, f.js)
	n.Require().NoError(err)
	s, err := NewSubscriber(f.consumer.Name, f.nc, f.js, f.consumer)
	n.Require().NoError(err)
	noop := func(context.Context, pubsub.Message, map[string]string) error { return nil }

	n.ErrorIs(p.Publish(context.Background(), "", []byte("x")), pubsub.ErrNilTopic)
	n.Error(s.Subscribe(context.Background(), "", noop))
	// subject not covered by any stream.
	n.Error(p.Publish(context.Background(), "kit.nostream."+id.New(), []byte("x")))
}

func (n *natsTestSuite) TestPublishAfterClose() {
	f := n.newFixture()
	p, err := NewPublisher(f.nc, f.js)
	n.Require().NoError(err)

	n.Require().NoError(p.Close())
	n.ErrorIs(p.Publish(context.Background(), f.subject, []byte("x")), pubsub.ErrPublisherClosed)
}

func (n *natsTestSuite) TestPublishAfterConnectionClosed() {
	f := n.newFixture()
	p, err := NewPublisher(f.nc, f.js)
	n.Require().NoError(err)

	f.nc.Close()
	n.ErrorIs(p.Publish(context.Background(), f.subject, []byte("x")), pubsub.ErrPublisherClosed)
	n.ErrorIs(p.Close(), pubsub.ErrPublisherClosed)
}

func (n *natsTestSuite) TestSubscriberCloseWithClosedConnection() {
	f := n.newFixture()
	s, err := NewSubscriber(f.consumer.Name, f.nc, f.js, f.consumer)
	n.Require().NoError(err)

	f.nc.Close()
	n.ErrorIs(s.Subscribe(context.Background(), f.subject, func(context.Context, pubsub.Message, map[string]string) error {
		return nil
	}), pubsub.ErrSubscriberClosed)
	n.ErrorIs(s.Close(), pubsub.ErrSubscriberClosed)
	n.NoError(s.Close(), "closing twice is a no-op")
}

func TestNewPublisherSubscriberValidation(t *testing.T) {
	nc := &nats.Conn{}
	js, err := nc.JetStream()
	require.NoError(t, err)
	consumer := &nats.ConsumerInfo{}

	pubTests := []struct {
		name string
		nc   *nats.Conn
		js   nats.JetStreamContext
	}{
		{name: "nil connection", nc: nil, js: js},
		{name: "nil jetstream", nc: nc, js: nil},
	}
	for _, tt := range pubTests {
		t.Run("publisher "+tt.name, func(t *testing.T) {
			p, err := NewPublisher(tt.nc, tt.js)
			assert.Error(t, err)
			assert.Nil(t, p)
		})
	}

	subTests := []struct {
		name     string
		group    string
		nc       *nats.Conn
		js       nats.JetStreamContext
		consumer *nats.ConsumerInfo
		wantErr  bool
	}{
		{name: "empty queue group", group: "", nc: nc, js: js, consumer: consumer, wantErr: true},
		{name: "nil connection", group: "g", nc: nil, js: js, consumer: consumer, wantErr: true},
		{name: "nil jetstream", group: "g", nc: nc, js: nil, consumer: consumer, wantErr: true},
		{name: "nil consumer", group: "g", nc: nc, js: js, consumer: nil, wantErr: true},
		{name: "valid", group: "g", nc: nc, js: js, consumer: consumer, wantErr: false},
	}
	for _, tt := range subTests {
		t.Run("subscriber "+tt.name, func(t *testing.T) {
			s, err := NewSubscriber(tt.group, tt.nc, tt.js, tt.consumer)
			if tt.wantErr {
				assert.Error(t, err)
				assert.Nil(t, s)
				return
			}
			assert.NoError(t, err)
			assert.NotNil(t, s)
		})
	}
}
