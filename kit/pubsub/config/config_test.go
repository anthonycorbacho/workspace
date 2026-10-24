package config

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/nats-io/nats.go"

	kitconfig "github.com/anthonycorbacho/workspace/kit/config"
	kitpubsub "github.com/anthonycorbacho/workspace/kit/pubsub"
	"github.com/anthonycorbacho/workspace/kit/pubsub/gcp"
	kitkafka "github.com/anthonycorbacho/workspace/kit/pubsub/kafka"
	kitmqtt "github.com/anthonycorbacho/workspace/kit/pubsub/mqtt"
	kitnats "github.com/anthonycorbacho/workspace/kit/pubsub/nats"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConfig(t *testing.T) {
	if os.Getenv("PUBSUB_EMULATOR_HOST") == "" {
		t.Skip("Skipping, no env variable PUBSUB_EMULATOR_HOST")
	}
	c := Config{
		Kind: "gcp-publisher",
		GCPPublisher: &GCPPublisher{
			GCPConnection: GCPConnection{Project: "fake"},
		},
	}

	p, _, err := c.Publisher(context.TODO())
	assert.NoError(t, err)
	assert.IsType(t, &gcp.Publisher{}, p)
}

func TestNATSSubConfig(t *testing.T) {
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
	err = kitconfig.From(rawConf, &c)
	if !assert.NoError(t, err) {
		return
	}
	p, close, err := c.Subscriber(context.TODO())
	defer close()
	assert.NoError(t, err)
	assert.IsType(t, &kitnats.Subscriber{}, p)
}

func TestNATSPubConfig(t *testing.T) {
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
	err = kitconfig.From(rawConf, &c)
	if !assert.NoError(t, err) {
		return
	}
	p, close, err := c.Publisher(context.TODO())
	defer close()
	assert.NoError(t, err)
	assert.IsType(t, &kitnats.Publisher{}, p)
}

func TestKafkaSASLOption(t *testing.T) {
	tests := []struct {
		mechanism string
		wantOpt   bool
		wantErr   bool
	}{
		{mechanism: "", wantOpt: false},
		{mechanism: "PLAIN", wantOpt: true},
		{mechanism: "SCRAM-SHA-256", wantOpt: true},
		{mechanism: "SCRAM-SHA-512", wantOpt: true},
		{mechanism: "OAUTHBEARER", wantErr: true},
	}
	for _, tc := range tests {
		t.Run(tc.mechanism, func(t *testing.T) {
			s := KafkaSASL{Mechanism: tc.mechanism, Username: "user", Password: "pwd"}
			opt, err := s.option()
			if tc.wantErr {
				assert.Error(t, err)
				return
			}
			assert.NoError(t, err)
			assert.Equal(t, tc.wantOpt, opt != nil)
		})
	}
}

func TestKafkaConfigFromYaml(t *testing.T) {
	rawConf := strings.NewReader(`kind: "kafka-subscriber"
kafkaSubscriber:
  brokers: ["broker1:9092", "broker2:9092"]
  consumerGroup: "my-group"
  sasl:
    mechanism: "PLAIN"
    username: "user"
    password: "pwd"`)
	c := Config{}
	err := kitconfig.From(rawConf, &c)
	if !assert.NoError(t, err) {
		return
	}
	assert.Equal(t, []string{"broker1:9092", "broker2:9092"}, c.KafkaSubscriber.Brokers)
	assert.Equal(t, "my-group", c.KafkaSubscriber.ConsumerGroup)
	assert.Equal(t, "PLAIN", c.KafkaSubscriber.SASL.Mechanism)
}

func TestKafkaRegexWithoutPatterns(t *testing.T) {
	c := Config{
		Kind: "kafka-subscriber",
		KafkaSubscriber: &KafkaSubscriber{
			Brokers:      []string{"localhost:9092"},
			RegexEnabled: true,
		},
	}
	_, closeFn, err := c.Subscriber(context.TODO())
	defer closeFn()
	assert.Error(t, err)
}

func TestKafkaConfig(t *testing.T) {
	if os.Getenv("TESTINGKAFKA_URL") == "" {
		t.Skip("Skipping, no env variable TESTINGKAFKA_URL")
	}
	brokers := []string{os.Getenv("TESTINGKAFKA_URL")}

	pc := Config{
		Kind:           "kafka-publisher",
		KafkaPublisher: &KafkaPublisher{Brokers: brokers},
	}
	p, closePub, err := pc.Publisher(context.TODO())
	defer closePub()
	assert.NoError(t, err)
	assert.IsType(t, &kitkafka.Publisher{}, p)

	sc := Config{
		Kind: "kafka-subscriber",
		KafkaSubscriber: &KafkaSubscriber{
			Brokers:       brokers,
			ConsumerGroup: "testsubconf",
		},
	}
	s, closeSub, err := sc.Subscriber(context.TODO())
	defer closeSub()
	assert.NoError(t, err)
	assert.IsType(t, &kitkafka.Subscriber{}, s)
}

func TestMQTTConfig(t *testing.T) {
	if os.Getenv("TESTINGMQTT_URL") == "" {
		t.Skip("Skipping, no env variable TESTINGMQTT_URL")
	}
	conn := MQTTConnection{Addr: os.Getenv("TESTINGMQTT_URL")}

	pc := Config{Kind: "mqtt-publisher", MQTTPublisher: &MQTTPublisher{MQTTConnection: conn}}
	p, closePub, err := pc.Publisher(context.TODO())
	defer closePub()
	assert.NoError(t, err)
	assert.IsType(t, &kitmqtt.Publisher{}, p)

	sc := Config{Kind: "mqtt-subscriber", MQTTSubscriber: &MQTTSubscriber{MQTTConnection: conn}}
	s, closeSub, err := sc.Subscriber(context.TODO())
	defer closeSub()
	assert.NoError(t, err)
	assert.IsType(t, &kitmqtt.Subscriber{}, s)
}

func TestGCPConfigCloseFunc(t *testing.T) {
	if os.Getenv("PUBSUB_EMULATOR_HOST") == "" {
		t.Skip("Skipping, no env variable PUBSUB_EMULATOR_HOST")
	}
	pc := Config{Kind: "gcp-publisher", GCPPublisher: &GCPPublisher{GCPConnection: GCPConnection{Project: "fake"}}}
	p, closePub, err := pc.Publisher(context.TODO())
	require.NoError(t, err)
	closePub()
	assert.ErrorIs(t, p.Publish(context.TODO(), "topic", []byte("msg")), kitpubsub.ErrPublisherClosed)

	sc := Config{Kind: "gcp-subscriber", GCPSubscriber: &GCPSubscriber{GCPConnection: GCPConnection{Project: "fake"}}}
	s, closeSub, err := sc.Subscriber(context.TODO())
	require.NoError(t, err)
	closeSub()
	assert.ErrorIs(t, s.Subscribe(context.TODO(), "sub", func(context.Context, kitpubsub.Message, map[string]string) error {
		return nil
	}), kitpubsub.ErrSubscriberClosed)
}

func TestNATSConfigCloseFunc(t *testing.T) {
	if os.Getenv("TESTINGNATS_URL") == "" {
		t.Skip("Skipping, no env variable TESTINGNATS_URL")
	}
	c := Config{
		Kind: "nats-publisher",
		NATSPublisher: &NATSPublisher{NATSConnection: NATSConnection{
			URL:                    os.Getenv("TESTINGNATS_URL"),
			PublishAsyncMaxPending: 16,
		}},
	}
	p, closePub, err := c.Publisher(context.TODO())
	require.NoError(t, err)
	closePub()
	assert.Error(t, p.Publish(context.TODO(), "subject", []byte("msg")))
}

func TestNATSConfigUnknownConsumer(t *testing.T) {
	if os.Getenv("TESTINGNATS_URL") == "" {
		t.Skip("Skipping, no env variable TESTINGNATS_URL")
	}
	c := Config{
		Kind: "nats-subscriber",
		NATSSubscriber: &NATSSubscriber{
			NATSConnection:    NATSConnection{URL: os.Getenv("TESTINGNATS_URL")},
			Stream:            "does-not-exist",
			ConsumerName:      "does-not-exist",
			ConsumerGroupName: "group",
		},
	}
	_, closeSub, err := c.Subscriber(context.TODO())
	defer closeSub()
	assert.ErrorContains(t, err, "failed to get consumer info")
}

func TestMQTTConfigUnreachableBroker(t *testing.T) {
	c := Config{Kind: "mqtt-publisher", MQTTPublisher: &MQTTPublisher{MQTTConnection: MQTTConnection{Addr: "mqtt://127.0.0.1:1"}}}
	_, closePub, err := c.Publisher(context.TODO())
	defer closePub()
	assert.Error(t, err)
}

func TestUnknownKind(t *testing.T) {
	c := Config{Kind: "unknown"}
	_, closePub, err := c.Publisher(context.TODO())
	closePub()
	assert.Error(t, err)
	_, closeSub, err := c.Subscriber(context.TODO())
	closeSub()
	assert.Error(t, err)

	missing := []Config{
		{Kind: "gcp-publisher"}, {Kind: "nats-publisher"}, {Kind: "kafka-publisher"}, {Kind: "mqtt-publisher"},
	}
	for _, c := range missing {
		_, _, err := c.Publisher(context.TODO())
		assert.Error(t, err, c.Kind)
	}
	missingSub := []Config{
		{Kind: "gcp-subscriber"}, {Kind: "nats-subscriber"}, {Kind: "kafka-subscriber"}, {Kind: "mqtt-subscriber"},
	}
	for _, c := range missingSub {
		_, _, err := c.Subscriber(context.TODO())
		assert.Error(t, err, c.Kind)
	}
}
