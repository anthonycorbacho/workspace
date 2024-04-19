package pubsubconfig

import (
	"context"
	"time"

	"cloud.google.com/go/pubsub"
	"github.com/anthonycorbacho/workspace/kit/config"
	"github.com/anthonycorbacho/workspace/kit/errors"
	"github.com/anthonycorbacho/workspace/kit/log"
	kitpubsub "github.com/anthonycorbacho/workspace/kit/pubsub"
	kitgcp "github.com/anthonycorbacho/workspace/kit/pubsub/gcp"
	kitnats "github.com/anthonycorbacho/workspace/kit/pubsub/nats"
	"github.com/nats-io/nats.go"
)

// Config represent a PubSub configuration, it is defined by its kind.
type Config struct {
	Kind           string          `yaml:"kind"`
	GcpPublisher   *GcpPublisher   `yaml:"gcpPublisher"`
	GcpSubscriber  *GcpSubscriber  `yaml:"gcpSubscriber"`
	NatsPublisher  *NatsPublisher  `yaml:"natsPublisher"`
	NatsSubscriber *NatsSubscriber `yaml:"natsSubscriber"`
}

func (c *Config) Publisher(ctx context.Context) (kitpubsub.Publisher, func(), error) {
	closeFn := func() {}
	switch c.Kind {
	case "gcp-publisher":
		if c.GcpPublisher == nil {
			return nil, closeFn, errors.New("gcp publisher missing")
		}
		client, err := pubsub.NewClient(ctx, c.GcpPublisher.Project)
		if err != nil {
			return nil, closeFn, err
		}
		pub, err := kitgcp.NewPublisher(client)
		return pub, closeFn, err
	case "nats-publisher":
		if c.NatsPublisher == nil {
			return nil, closeFn, errors.New("nats publisher missing")
		}
		conf := c.NatsPublisher
		con, js, err := natsConnection(&conf.NatsConnection)
		if err != nil {
			return nil, closeFn, errors.Wrap(err, "")
		}
		pub, err := kitnats.NewPublisher(con, js)
		return pub, closeFn, err
	}

	return nil, closeFn, errors.New("unknown pubsub provider")
}

func (c *Config) Subscriber(ctx context.Context) (kitpubsub.Subscriber, func(), error) {
	closeFn := func() {}
	switch c.Kind {
	case "gcp-subscriber":
		if c.GcpSubscriber == nil {
			return nil, closeFn, errors.New("gcp subscriber missing")
		}
		client, err := pubsub.NewClient(ctx, c.GcpSubscriber.Project)
		if err != nil {
			return nil, closeFn, err
		}
		sub, err := kitgcp.NewSubscriber(client, c.GcpSubscriber.withOptions()...)
		return sub, closeFn, err
	case "nats-subscriber":
		if c.NatsSubscriber == nil {
			return nil, closeFn, errors.New("nats subscriber missing")
		}
		conf := c.NatsSubscriber
		con, js, err := natsConnection(&conf.NatsConnection)
		if err != nil {
			return nil, closeFn, errors.Wrap(err, "")
		}
		cons, err := js.ConsumerInfo(conf.Stream, conf.ConsumerName)
		if err != nil {
			return nil, closeFn, errors.Wrap(err, "failed to get consumer info")
		}

		sub, err := kitnats.NewSubscriber(conf.ConsumerGroupName, con, js, cons)
		if err != nil {
			return nil, closeFn, errors.Wrap(err, "failed to get consumer info")
		}
		closeFn = func() {
			err := sub.Close()
			con.Close()
			if err != nil {
				log.L().Warn(ctx, "failed to close nats sub", log.Error(err))
			}
		}
		return sub, closeFn, nil
	}

	return nil, closeFn, errors.New("unknown pubsub subscriber")
}

type GcpPublisher struct {
	Project string `yaml:"project"`
}

type GcpSubscriber struct {
	GcpPublisher           `yaml:",inline"`
	MaxExtension           time.Duration `yaml:"maxExtension"`
	MaxExtensionPeriod     time.Duration `yaml:"maxExtensionPeriod"`
	MinExtensionPeriod     time.Duration `yaml:"minExtensionPeriod"`
	NumGoroutines          int           `yaml:"numGoroutines"`
	MaxOutstandingBytes    int           `yaml:"maxOutstandingBytes"`
	MaxOutstandingMessages int           `yaml:"maxOutstandingMessages"`
}

func (gcpsub *GcpSubscriber) withOptions() []kitgcp.SubscriberOption {
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

type NatsConnection struct {
	URL string
	// when we connect to nats-mqtt we might need the mqtt passwords
	MqttPasswords          string `env:"NATS_MQTT_TOKEN,overwrite"`
	PublishAsyncMaxPending int    `env:"NATS_JS_MAX_ASYNC_PUBLISH,overwrite,default=256"`
}

func natsConnection(conf *NatsConnection) (*nats.Conn, nats.JetStreamContext, error) {
	// POD_NAME should be configure for all our pod, and it is codegen
	// if missing it will just remove some metadata that are handy to debug
	podName := config.LookupEnv("POD_NAME", "")
	// TODO: should we should save nats connection to reuse them instead of creating new one
	natsOpts := []nats.Option{nats.Name(podName)}
	if conf.MqttPasswords != "" {
		natsOpts = append(natsOpts, nats.UserInfo(conf.MqttPasswords, ""))
	}
	con, err := nats.Connect(
		conf.URL,
		natsOpts...,
	)
	if err != nil {
		return nil, nil, errors.Wrap(err, "failed to connect to nats")
	}
	// TODO: should we should save js connection to reuse them instead of creating new one
	jsOpts := []nats.JSOpt{}
	if conf.PublishAsyncMaxPending > 0 {
		jsOpts = append(jsOpts, nats.PublishAsyncMaxPending(256))
	}
	js, err := con.JetStream(jsOpts...)
	if err != nil {
		return nil, nil, errors.Wrap(err, "failed to get jetstream")
	}
	return con, js, nil
}

// NatsPublisher is a nats publisher configuration
// To create publisher use our k8s ressources https://github.com/nats-io/nack
type NatsPublisher struct {
	NatsConnection `yaml:",inline"`
}

// NatsSubscriber is a nats subscriber configuration
// To create subscriber use our k8s ressources https://github.com/nats-io/nack
type NatsSubscriber struct {
	NatsConnection    `yaml:",inline"`
	Stream            string `yaml:"stream"`
	ConsumerName      string `yaml:"consumerName"`
	ConsumerGroupName string `yaml:"consumerGroupName"`
}
