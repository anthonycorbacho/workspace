package gcp

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	gcppubsub "cloud.google.com/go/pubsub/v2"
	"cloud.google.com/go/pubsub/v2/apiv1/pubsubpb"
	"github.com/anthonycorbacho/workspace/kit/id"
	"github.com/anthonycorbacho/workspace/kit/pubsub"
	"github.com/stretchr/testify/suite"
)

const testProject = "fake"

type gcpTestSuite struct {
	suite.Suite
	admin *gcppubsub.Client
}

func TestGcpTestSuite(t *testing.T) {
	suite.Run(t, new(gcpTestSuite))
}

func (g *gcpTestSuite) SetupSuite() {
	if os.Getenv("PUBSUB_EMULATOR_HOST") == "" {
		g.T().Skip("Skipping, no testing gcp emulator setup via env variable PUBSUB_EMULATOR_HOST")
	}
	c, err := gcppubsub.NewClient(context.Background(), testProject)
	g.Require().NoError(err)
	g.admin = c
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
		choke <- received{msg: msg.String(), metadata: metadata, topic: pubsub.GetTopic(ctx)}
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
