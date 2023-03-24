package nats

import (
	"context"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/anthonycorbacho/workspace/kit/log"
	"github.com/anthonycorbacho/workspace/kit/pubsub"
	"github.com/nats-io/nats.go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
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

func TestNatsTestSuite(t *testing.T) {
	suite.Run(t, new(natsTestSuite))
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

	js, nc, err := New(addr, errorHandler(errCh), closedHandler(&wg))
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
	n.s.Subscribe(ctx, testDefaultSubject, func(ctx context.Context, msg pubsub.Message) error {
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

func (n *natsTestSuite) checkHeaders(msg *nats.Msg) {
	expectedHeaders := [5]string{"subject", "trace", "span", "trace-state", "trace-remote"}
	for _, h := range expectedHeaders {
		_, exists := msg.Header[h]
		if exists != true {
			assert.Fail(n.T(), fmt.Sprintf("header %s isn't in the message header", h))
		}
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
	js1, nc1, err := New(addr)
	if err != nil {
		n.T().Fatalf("setting up nats server failed: %v", err)
	}
	p, err := NewPublisher(nc1, js1)
	if err != nil {
		n.T().Fatalf("setting up publisher: %v", err)
	}
	// Subscriber
	js2, nc2, err := New(addr)
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
	s.Subscribe(ctx, testClosingSubject, func(ctx context.Context, msg pubsub.Message) error {
		fmt.Println("Message received")
		fmt.Println(msg)
		ch <- fmt.Sprintf("Message received: %v", string(msg))
		return nil
	})
	err = s.Close()
	assert.NoError(n.T(), err)
	err = s.Close()
	assert.Error(n.T(), pubsub.SubscriberCLosed, err)

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
	assert.Error(n.T(), pubsub.PublisherClosed, err)
}
