package pubsubconfig

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/nats-io/nats.go"

	"github.com/anthonycorbacho/workspace/kit/config"
	"github.com/anthonycorbacho/workspace/kit/pubsub/gcp"
	kitnats "github.com/anthonycorbacho/workspace/kit/pubsub/nats"
	"github.com/stretchr/testify/assert"
)

func TestConfig(t *testing.T) {
	if os.Getenv("PUBSUB_EMULATOR_HOST") == "" {
		t.Skip("Skipping, no env variable PUBSUB_EMULATOR_HOST")
	}
	c := Config{
		Kind: "gcp-publisher",
		GcpPublisher: &GcpPublisher{
			Project: "fake",
		},
	}

	p, _, err := c.Publisher(context.TODO())
	assert.NoError(t, err)
	assert.IsType(t, &gcp.Publisher{}, p)
}

func TestNatsSubConfig(t *testing.T) {
	if os.Getenv("TESTINGNATS_URL") == "" {
		t.Skip("Skipping, no env variable TESTINGNATS_URL")
	}
	// we need to prepare jetstream for the test
	con, err := nats.Connect(os.Getenv("TESTINGNATS_URL"))
	if !assert.NoError(t, err) {
		return
	}
	js, err := con.JetStream()
	if !assert.NoError(t, err) {
		return
	}
	_, err = js.AddStream(&nats.StreamConfig{
		Name: "testsubconf",
	})
	if !assert.NoError(t, err) {
		return
	}
	_, err = js.AddConsumer("testsubconf", &nats.ConsumerConfig{
		Durable:   "testsubconfconsumer",
		AckPolicy: nats.AckExplicitPolicy,
	})
	if !assert.NoError(t, err) {
		return
	}

	// ensure connection working well
	rawConf := strings.NewReader(`kind: "nats-subscriber"
natsSubscriber:
  url: "nats://127.0.0.1:4222"
  stream: "testsubconf"
  consumerName: "testsubconfconsumer"
  consumerGroupName: "cc"`)
	c := Config{}
	// we use config from to ensure that the OS env is well respected for podName
	err = config.From(rawConf, &c)
	if !assert.NoError(t, err) {
		return
	}
	p, close, err := c.Subscriber(context.TODO())
	defer close()
	assert.NoError(t, err)
	assert.IsType(t, &kitnats.Subscriber{}, p)
}

func TestNatsPubConfig(t *testing.T) {
	if os.Getenv("TESTINGNATS_URL") == "" {
		t.Skip("Skipping, no env variable TESTINGNATS_URL")
	}
	// we need to prepare jetstream for the test
	con, err := nats.Connect(os.Getenv("TESTINGNATS_URL"))
	if !assert.NoError(t, err) {
		return
	}
	js, err := con.JetStream()
	if !assert.NoError(t, err) {
		return
	}
	_, err = js.AddStream(&nats.StreamConfig{
		Name: "testpubconf",
	})
	if !assert.NoError(t, err) {
		return
	}

	// ensure connection working well
	rawConf := strings.NewReader(`kind: "nats-publisher"
natsPublisher:
  url: "nats://127.0.0.1:4222"`)
	c := Config{}
	// we use config from to ensure that the OS env is well respected for podName
	err = config.From(rawConf, &c)
	if !assert.NoError(t, err) {
		return
	}
	p, close, err := c.Publisher(context.TODO())
	defer close()
	assert.NoError(t, err)
	assert.IsType(t, &kitnats.Publisher{}, p)
}
