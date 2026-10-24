package config

import (
	"context"
	"time"

	"cloud.google.com/go/pubsub/v2"
	kitconfig "github.com/anthonycorbacho/workspace/kit/config"
	"github.com/anthonycorbacho/workspace/kit/errors"
	"github.com/anthonycorbacho/workspace/kit/id"
	"github.com/anthonycorbacho/workspace/kit/log"
	kitpubsub "github.com/anthonycorbacho/workspace/kit/pubsub"
	kitgcp "github.com/anthonycorbacho/workspace/kit/pubsub/gcp"
	kitnats "github.com/anthonycorbacho/workspace/kit/pubsub/nats"
	"github.com/nats-io/nats.go"
)

// Config is a pubsub configuration, it is defined by its kind, the configuration of the kind
// must be set:
//
//   - gcp-publisher (GCPPublisher), gcp-subscriber (GCPSubscriber): Google Cloud Pub/Sub.
//   - nats-publisher (NATSPublisher), nats-subscriber (NATSSubscriber): NATS JetStream.
//   - kafka-publisher (KafkaPublisher), kafka-subscriber (KafkaSubscriber): Kafka.
//   - mqtt-publisher (MQTTPublisher), mqtt-subscriber (MQTTSubscriber): MQTT v5.
type Config struct {
	Kind            string           `yaml:"kind"`
	GCPPublisher    *GCPPublisher    `yaml:"gcpPublisher"`
	GCPSubscriber   *GCPSubscriber   `yaml:"gcpSubscriber"`
	NATSPublisher   *NATSPublisher   `yaml:"natsPublisher"`
	NATSSubscriber  *NATSSubscriber  `yaml:"natsSubscriber"`
	KafkaPublisher  *KafkaPublisher  `yaml:"kafkaPublisher"`
	KafkaSubscriber *KafkaSubscriber `yaml:"kafkaSubscriber"`
	MQTTPublisher   *MQTTPublisher   `yaml:"mqttPublisher"`
	MQTTSubscriber  *MQTTSubscriber  `yaml:"mqttSubscriber"`
}

// Publisher creates the publisher defined by the configuration kind, ctx bounds the creation of
// its client (when the client supports it) and its values are used by the logs of the client.
// The returned function must be called to release the publisher resources.
func (c *Config) Publisher(ctx context.Context) (kitpubsub.Publisher, func(), error) {
	noop := func() {}
	switch c.Kind {
	case "gcp-publisher":
		if c.GCPPublisher == nil {
			return nil, noop, errors.New("gcp publisher missing")
		}
		return c.GCPPublisher.publisher(ctx)
	case "nats-publisher":
		if c.NATSPublisher == nil {
			return nil, noop, errors.New("nats publisher missing")
		}
		return c.NATSPublisher.publisher(ctx)
	case "kafka-publisher":
		if c.KafkaPublisher == nil {
			return nil, noop, errors.New("kafka publisher missing")
		}
		return c.KafkaPublisher.publisher(ctx)
	case "mqtt-publisher":
		if c.MQTTPublisher == nil {
			return nil, noop, errors.New("mqtt publisher missing")
		}
		return c.MQTTPublisher.publisher(ctx)
	}

	return nil, noop, errors.New("unknown pubsub provider")
}

// Subscriber creates the subscriber defined by the configuration kind, ctx bounds the creation of
// its client (when the client supports it) and its values are used by the logs of the client.
// The returned function must be called to release the subscriber resources.
func (c *Config) Subscriber(ctx context.Context) (kitpubsub.Subscriber, func(), error) {
	noop := func() {}
	switch c.Kind {
	case "gcp-subscriber":
		if c.GCPSubscriber == nil {
			return nil, noop, errors.New("gcp subscriber missing")
		}
		return c.GCPSubscriber.subscriber(ctx)
	case "nats-subscriber":
		if c.NATSSubscriber == nil {
			return nil, noop, errors.New("nats subscriber missing")
		}
		return c.NATSSubscriber.subscriber(ctx)
	case "kafka-subscriber":
		if c.KafkaSubscriber == nil {
			return nil, noop, errors.New("kafka subscriber missing")
		}
		return c.KafkaSubscriber.subscriber(ctx)
	case "mqtt-subscriber":
		if c.MQTTSubscriber == nil {
			return nil, noop, errors.New("mqtt subscriber missing")
		}
		return c.MQTTSubscriber.subscriber(ctx)
	}

	return nil, noop, errors.New("unknown pubsub subscriber")
}

// clientID returns a unique client identifier for brokers that require one (kafka, mqtt).
// It must be unique per connection since mqtt brokers disconnect clients sharing the same id,
// POD_NAME is used as prefix when available to ease debugging.
func clientID(suffix string) string {
	if podName := kitconfig.LookupEnv("POD_NAME", ""); podName != "" {
		return podName + "-" + suffix + "-" + id.New()
	}
	return suffix + "-" + id.New()
}

// GCPConnection is a Google Cloud Pub/Sub connection configuration, shared by the publisher and
// the subscriber. The client connects to the emulator when PUBSUB_EMULATOR_HOST is set.
type GCPConnection struct {
	Project string `yaml:"project"`
}

// GCPPublisher is a Google Cloud Pub/Sub publisher configuration.
type GCPPublisher struct {
	GCPConnection `yaml:",inline"`
}

// GCPSubscriber is a Google Cloud Pub/Sub subscriber configuration, the durations and sizes not
// set (zero) use the default receive settings (see the With* options of kit/pubsub/gcp).
type GCPSubscriber struct {
	GCPConnection          `yaml:",inline"`
	MaxExtension           time.Duration `yaml:"maxExtension"`
	MaxExtensionPeriod     time.Duration `yaml:"maxExtensionPeriod"`
	MinExtensionPeriod     time.Duration `yaml:"minExtensionPeriod"`
	NumGoroutines          int           `yaml:"numGoroutines"`
	MaxOutstandingBytes    int           `yaml:"maxOutstandingBytes"`
	MaxOutstandingMessages int           `yaml:"maxOutstandingMessages"`
}

func (gcppub *GCPPublisher) publisher(ctx context.Context) (kitpubsub.Publisher, func(), error) {
	noop := func() {}
	client, err := pubsub.NewClient(ctx, gcppub.Project)
	if err != nil {
		return nil, noop, errors.Wrap(err, "creating gcp client")
	}
	pub, err := kitgcp.NewPublisher(client)
	if err != nil {
		closeClient(ctx, client.Close)
		return nil, noop, err
	}
	return pub, closer(ctx, "gcp publisher", pub.Close), nil
}

func (gcpsub *GCPSubscriber) subscriber(ctx context.Context) (kitpubsub.Subscriber, func(), error) {
	noop := func() {}
	client, err := pubsub.NewClient(ctx, gcpsub.Project)
	if err != nil {
		return nil, noop, errors.Wrap(err, "creating gcp client")
	}
	sub, err := kitgcp.NewSubscriber(client, gcpsub.withOptions()...)
	if err != nil {
		closeClient(ctx, client.Close)
		return nil, noop, err
	}
	return sub, closer(ctx, "gcp subscriber", sub.Close), nil
}

func (gcpsub *GCPSubscriber) withOptions() []kitgcp.SubscriberOption {
	opts := make([]kitgcp.SubscriberOption, 0, 6)

	if gcpsub.MaxExtension > 0 {
		opts = append(opts, kitgcp.WithMaxExtension(gcpsub.MaxExtension))
	}
	if gcpsub.MaxExtensionPeriod > 0 {
		opts = append(opts, kitgcp.WithMaxExtensionPeriod(gcpsub.MaxExtensionPeriod))
	}
	if gcpsub.NumGoroutines > 0 {
		opts = append(opts, kitgcp.WithNumGoroutines(gcpsub.NumGoroutines))
	}
	if gcpsub.MaxOutstandingMessages > 0 {
		opts = append(opts, kitgcp.WithMaxOutstandingMessages(gcpsub.MaxOutstandingMessages))
	}
	if gcpsub.MinExtensionPeriod > 0 {
		opts = append(opts, kitgcp.WithMinExtensionPeriod(gcpsub.MinExtensionPeriod))
	}
	if gcpsub.MaxOutstandingBytes > 0 {
		opts = append(opts, kitgcp.WithMaxOutstandingBytes(gcpsub.MaxOutstandingBytes))
	}

	return opts
}

// natsRequestTimeout is the timeout of the NATS requests, the JetStream default.
const natsRequestTimeout = 5 * time.Second

// NATSConnection is a NATS connection configuration, shared by the publisher and the subscriber.
type NATSConnection struct {
	URL string `yaml:"url"`
	// Token authenticates the connection, it is sent as the connection user name with an empty
	// password (eg: the token of a NATS server with MQTT enabled).
	Token string `yaml:"token" env:"NATS_MQTT_TOKEN,overwrite"`
	// PublishAsyncMaxPending is the maximum number of pending asynchronous JetStream publishes.
	PublishAsyncMaxPending int `env:"NATS_JS_MAX_ASYNC_PUBLISH,overwrite,default=256"`
}

// natsConnection connects to NATS and returns the connection and its JetStream context.
func natsConnection(conf *NATSConnection) (*nats.Conn, nats.JetStreamContext, error) {
	// POD_NAME should be configure for all our pod, and it is codegen
	// if missing it will just remove some metadata that are handy to debug
	podName := kitconfig.LookupEnv("POD_NAME", "")
	// TODO: should we should save nats connection to reuse them instead of creating new one
	natsOpts := []nats.Option{nats.Name(podName)}
	if conf.Token != "" {
		natsOpts = append(natsOpts, nats.UserInfo(conf.Token, ""))
	}
	con, err := nats.Connect(
		conf.URL,
		natsOpts...,
	)
	if err != nil {
		return nil, nil, errors.Wrap(err, "failed to connect to nats")
	}
	// TODO: should we should save js connection to reuse them instead of creating new one
	var jsOpts []nats.JSOpt
	if conf.PublishAsyncMaxPending > 0 {
		jsOpts = append(jsOpts, nats.PublishAsyncMaxPending(conf.PublishAsyncMaxPending))
	}
	js, err := con.JetStream(jsOpts...)
	if err != nil {
		con.Close()
		return nil, nil, errors.Wrap(err, "failed to get jetstream")
	}
	return con, js, nil
}

// NATSPublisher is a NATS JetStream publisher configuration.
// The streams are created with the k8s resources of https://github.com/nats-io/nack.
type NATSPublisher struct {
	NATSConnection `yaml:",inline"`
}

// NATSSubscriber is a NATS JetStream subscriber configuration.
// The streams and consumers are created with the k8s resources of https://github.com/nats-io/nack.
type NATSSubscriber struct {
	NATSConnection `yaml:",inline"`
	// Stream is the stream of the consumer.
	Stream string `yaml:"stream"`
	// ConsumerName is the durable consumer the subscriptions are bound to.
	ConsumerName string `yaml:"consumerName"`
	// ConsumerGroupName is the queue group the subscriptions join.
	ConsumerGroupName string `yaml:"consumerGroupName"`
}

func (natspub *NATSPublisher) publisher(ctx context.Context) (kitpubsub.Publisher, func(), error) {
	noop := func() {}
	con, js, err := natsConnection(&natspub.NATSConnection)
	if err != nil {
		return nil, noop, err
	}
	pub, err := kitnats.NewPublisher(con, js)
	if err != nil {
		con.Close()
		return nil, noop, err
	}
	// Close drains the connection, pending messages are published before the connection is closed.
	return pub, closer(ctx, "nats publisher", pub.Close), nil
}

func (natssub *NATSSubscriber) subscriber(ctx context.Context) (kitpubsub.Subscriber, func(), error) {
	noop := func() {}
	con, js, err := natsConnection(&natssub.NATSConnection)
	if err != nil {
		return nil, noop, err
	}
	// bounded like the JetStream requests without context.
	infoCtx, cancel := context.WithTimeout(ctx, natsRequestTimeout)
	defer cancel()
	cons, err := js.ConsumerInfo(natssub.Stream, natssub.ConsumerName, nats.Context(infoCtx))
	if err != nil {
		con.Close()
		return nil, noop, errors.Wrap(err, "failed to get consumer info")
	}

	sub, err := kitnats.NewSubscriber(natssub.ConsumerGroupName, con, js, cons)
	if err != nil {
		con.Close()
		return nil, noop, errors.Wrap(err, "failed to create nats subscriber")
	}
	closeFn := func() {
		// Close drains the connection and closes it once drained,
		// the connection is only closed here when draining failed.
		if err := sub.Close(); err != nil {
			log.L().Warn(ctx, "failed to close nats sub", log.Error(err))
			con.Close()
		}
	}
	return sub, closeFn, nil
}

// closer returns a function closing a publisher or subscriber and logging the failure,
// closing is done on shutdown where the error can't be handled by the caller.
func closer(ctx context.Context, name string, closeFn func() error) func() {
	return func() {
		if err := closeFn(); err != nil {
			log.L().Warn(ctx, "failed to close "+name, log.Error(err))
		}
	}
}

// closeClient closes a client on a constructor error path, the constructor error is returned instead.
func closeClient(ctx context.Context, closeFn func() error) {
	if err := closeFn(); err != nil {
		log.L().Warn(ctx, "failed to close client", log.Error(err))
	}
}
