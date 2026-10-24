package config

import (
	"context"

	"github.com/anthonycorbacho/workspace/kit/errors"
	kitpubsub "github.com/anthonycorbacho/workspace/kit/pubsub"
	kitmqtt "github.com/anthonycorbacho/workspace/kit/pubsub/mqtt"
)

// MQTTConnection is a MQTT connection configuration, shared by the publisher and the subscriber.
type MQTTConnection struct {
	// Addr is the broker address.
	Addr string `yaml:"addr" env:"MQTT_ADDR,overwrite,default=mqtt://localhost:1883"`
	// User and Password authenticate the connection when User is set.
	User     string `yaml:"user" env:"MQTT_USER,overwrite"`
	Password string `yaml:"password" env:"MQTT_PASSWORD,overwrite"`
}

// MQTTPublisher is a mqtt publisher configuration.
type MQTTPublisher struct {
	MQTTConnection `yaml:",inline"`
}

// MQTTSubscriber is a mqtt subscriber configuration.
type MQTTSubscriber struct {
	MQTTConnection `yaml:",inline"`
}

func (c *MQTTConnection) client(ctx context.Context, clientID string) (*kitmqtt.Client, error) {
	var opts []kitmqtt.Option
	if len(c.User) != 0 {
		opts = append(opts, kitmqtt.WithUser(c.User), kitmqtt.WithPassword(c.Password))
	}
	return kitmqtt.NewClient(ctx, c.Addr, clientID, opts...)
}

func (m *MQTTPublisher) publisher(ctx context.Context) (kitpubsub.Publisher, func(), error) {
	noop := func() {}
	client, err := m.client(ctx, clientID("publisher"))
	if err != nil {
		return nil, noop, errors.Wrap(err, "failed to create mqtt client")
	}
	pub, err := kitmqtt.NewPublisher(client)
	if err != nil {
		closeClient(ctx, func() error { return client.Disconnect(ctx) })
		return nil, noop, err
	}
	return pub, closer(ctx, "mqtt publisher", pub.Close), nil
}

func (m *MQTTSubscriber) subscriber(ctx context.Context) (kitpubsub.Subscriber, func(), error) {
	noop := func() {}
	client, err := m.client(ctx, clientID("subscriber"))
	if err != nil {
		return nil, noop, errors.Wrap(err, "failed to create mqtt client")
	}
	sub, err := kitmqtt.NewSubscriber(client)
	if err != nil {
		closeClient(ctx, func() error { return client.Disconnect(ctx) })
		return nil, noop, err
	}
	return sub, closer(ctx, "mqtt subscriber", sub.Close), nil
}
