package pubsubconfig

import (
	"context"

	"github.com/anthonycorbacho/workspace/kit/errors"
	"github.com/anthonycorbacho/workspace/kit/log"
	kitpubsub "github.com/anthonycorbacho/workspace/kit/pubsub"
	kitmqtt "github.com/anthonycorbacho/workspace/kit/pubsub/mqtt"
)

// MqttConnection is a mqtt connection configuration.
type MqttConnection struct {
	Addr     string `yaml:"addr" env:"MQTT_ADDR,overwrite,default=mqtt://localhost:1883"`
	User     string `yaml:"user" env:"MQTT_USER,overwrite"`
	Password string `yaml:"password" env:"MQTT_PASSWORD,overwrite"`
}

// MqttPublisher is a mqtt publisher configuration.
type MqttPublisher struct {
	MqttConnection `yaml:",inline"`
}

// MqttSubscriber is a mqtt subscriber configuration.
type MqttSubscriber struct {
	MqttConnection `yaml:",inline"`
}

func (c *MqttConnection) client(clientID string) (*kitmqtt.Client, error) {
	var opts []kitmqtt.Option
	if len(c.User) != 0 {
		opts = append(opts, kitmqtt.WithUser(c.User), kitmqtt.WithPassword(c.Password))
	}
	return kitmqtt.NewClient(c.Addr, clientID, opts...)
}

func (m *MqttPublisher) publisher(ctx context.Context) (kitpubsub.Publisher, func(), error) {
	noop := func() {}
	client, err := m.client(clientID("publisher"))
	if err != nil {
		return nil, noop, errors.Wrap(err, "failed to create mqtt client")
	}
	pub, err := kitmqtt.NewPublisher(client)
	if err != nil {
		closeClient(ctx, func() error { return client.Disconnect(ctx) })
		return nil, noop, err
	}
	closeFn := func() {
		if err := pub.Close(); err != nil {
			log.L().Warn(ctx, "failed to close mqtt pub", log.Error(err))
		}
	}
	return pub, closeFn, nil
}

func (m *MqttSubscriber) subscriber(ctx context.Context) (kitpubsub.Subscriber, func(), error) {
	noop := func() {}
	client, err := m.client(clientID("subscriber"))
	if err != nil {
		return nil, noop, errors.Wrap(err, "failed to create mqtt client")
	}
	sub, err := kitmqtt.NewSubscriber(client)
	if err != nil {
		closeClient(ctx, func() error { return client.Disconnect(ctx) })
		return nil, noop, err
	}
	closeFn := func() {
		if err := sub.Close(); err != nil {
			log.L().Warn(ctx, "failed to close mqtt sub", log.Error(err))
		}
	}
	return sub, closeFn, nil
}
