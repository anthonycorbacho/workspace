package pubsubconfig

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/nats-io/nats.go"

	"github.com/anthonycorbacho/workspace/kit/config"
	"github.com/anthonycorbacho/workspace/kit/pubsub/gcp"
	kitkafka "github.com/anthonycorbacho/workspace/kit/pubsub/kafka"
	kitmqtt "github.com/anthonycorbacho/workspace/kit/pubsub/mqtt"
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

func TestKafkaSaslOption(t *testing.T) {
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
			s := KafkaSasl{Mechanism: tc.mechanism, Username: "user", Password: "pwd"}
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
	err := config.From(rawConf, &c)
	if !assert.NoError(t, err) {
		return
	}
	assert.Equal(t, []string{"broker1:9092", "broker2:9092"}, c.KafkaSubscriber.Brokers)
	assert.Equal(t, "my-group", c.KafkaSubscriber.ConsumerGroup)
	assert.Equal(t, "PLAIN", c.KafkaSubscriber.Sasl.Mechanism)
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

func TestMqttConfig(t *testing.T) {
	if os.Getenv("TESTINGMQTT_URL") == "" {
		t.Skip("Skipping, no env variable TESTINGMQTT_URL")
	}
	conn := MqttConnection{Addr: os.Getenv("TESTINGMQTT_URL")}

	pc := Config{Kind: "mqtt-publisher", MqttPublisher: &MqttPublisher{MqttConnection: conn}}
	p, closePub, err := pc.Publisher(context.TODO())
	defer closePub()
	assert.NoError(t, err)
	assert.IsType(t, &kitmqtt.Publisher{}, p)

	sc := Config{Kind: "mqtt-subscriber", MqttSubscriber: &MqttSubscriber{MqttConnection: conn}}
	s, closeSub, err := sc.Subscriber(context.TODO())
	defer closeSub()
	assert.NoError(t, err)
	assert.IsType(t, &kitmqtt.Subscriber{}, s)
}
