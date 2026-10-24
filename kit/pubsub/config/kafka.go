package config

import (
	"context"

	"github.com/anthonycorbacho/workspace/kit/errors"
	kitpubsub "github.com/anthonycorbacho/workspace/kit/pubsub"
	kitkafka "github.com/anthonycorbacho/workspace/kit/pubsub/kafka"
)

// KafkaConnection is a kafka connection configuration, shared by the publisher and the subscriber.
type KafkaConnection struct {
	Brokers []string  `yaml:"brokers" env:"KAFKA_BROKERS,overwrite,default=localhost:9092"`
	SASL    KafkaSASL `yaml:"sasl"`
}

// KafkaSASL is a kafka SASL authentication configuration.
// Supported mechanisms are PLAIN, SCRAM-SHA-256 and SCRAM-SHA-512.
type KafkaSASL struct {
	Mechanism string `yaml:"mechanism" env:"KAFKA_SASL_MECHANISM,overwrite"`
	Username  string `yaml:"username" env:"KAFKA_SASL_USERNAME,overwrite"`
	Password  string `yaml:"password" env:"KAFKA_SASL_PASSWORD,overwrite"`
}

// KafkaPublisher is a kafka publisher configuration.
type KafkaPublisher struct {
	KafkaConnection `yaml:",inline"`
	// AutoTopicCreation allows the broker to create missing topics on publish.
	AutoTopicCreation bool `yaml:"autoTopicCreation" env:"KAFKA_AUTO_TOPIC_CREATION,overwrite"`
}

// KafkaSubscriber is a kafka subscriber configuration.
type KafkaSubscriber struct {
	KafkaConnection `yaml:",inline"`
	// ConsumerGroup is the consumer group the subscriber joins, required to commit the offsets.
	ConsumerGroup string `yaml:"consumerGroup" env:"KAFKA_CONSUMER_GROUP,overwrite"`
	// AutoCommitEnabled commits the acked records periodically instead of on each ack.
	AutoCommitEnabled bool `yaml:"autoCommitEnabled" env:"KAFKA_AUTO_COMMIT_ENABLED,overwrite"`
	// RegexEnabled consumes the topics matching RegexPatterns, the patterns must be set up front.
	RegexEnabled bool `yaml:"regexEnabled" env:"KAFKA_REGEX_ENABLED,overwrite"`
	// RegexPatterns are the patterns of the consumed topics when RegexEnabled is set.
	RegexPatterns []string `yaml:"regexPatterns" env:"KAFKA_REGEX_PATTERNS,overwrite"`
}

func (s *KafkaSASL) option() (kitkafka.Option, error) {
	switch s.Mechanism {
	case "":
		return nil, nil
	case "PLAIN":
		return kitkafka.WithSASLPlain(s.Username, s.Password), nil
	case "SCRAM-SHA-256":
		return kitkafka.WithSASLScramSHA256(s.Username, s.Password), nil
	case "SCRAM-SHA-512":
		return kitkafka.WithSASLScramSHA512(s.Username, s.Password), nil
	}
	return nil, errors.Newf("unknown kafka sasl mechanism: %s", s.Mechanism)
}

func (c *KafkaConnection) options(clientID string) ([]kitkafka.Option, error) {
	opts := []kitkafka.Option{kitkafka.WithClientID(clientID)}
	sasl, err := c.SASL.option()
	if err != nil {
		return nil, err
	}
	if sasl != nil {
		opts = append(opts, sasl)
	}
	return opts, nil
}

func (k *KafkaPublisher) publisher(ctx context.Context) (kitpubsub.Publisher, func(), error) {
	noop := func() {}
	opts, err := k.options(clientID("publisher"))
	if err != nil {
		return nil, noop, err
	}
	if k.AutoTopicCreation {
		opts = append(opts, kitkafka.WithAutoTopicCreation())
	}
	client, err := kitkafka.NewClient(k.Brokers, opts...)
	if err != nil {
		return nil, noop, errors.Wrap(err, "failed to create kafka client")
	}
	pub, err := kitkafka.NewPublisher(client)
	if err != nil {
		client.Close()
		return nil, noop, err
	}
	return pub, closer(ctx, "kafka publisher", pub.Close), nil
}

func (k *KafkaSubscriber) subscriber(ctx context.Context) (kitpubsub.Subscriber, func(), error) {
	noop := func() {}
	opts, err := k.options(clientID("subscriber"))
	if err != nil {
		return nil, noop, err
	}
	if k.ConsumerGroup != "" {
		opts = append(opts, kitkafka.WithConsumerGroup(k.ConsumerGroup))
	}
	if k.AutoCommitEnabled {
		opts = append(opts, kitkafka.WithAutoCommit())
	}
	if k.RegexEnabled {
		if len(k.RegexPatterns) == 0 {
			return nil, noop, errors.New("kafka regex enabled but no regex patterns given")
		}
		opts = append(opts, kitkafka.WithRegex(k.RegexPatterns))
	}
	client, err := kitkafka.NewClient(k.Brokers, opts...)
	if err != nil {
		return nil, noop, errors.Wrap(err, "failed to create kafka client")
	}
	sub, err := kitkafka.NewSubscriber(client)
	if err != nil {
		client.Close()
		return nil, noop, err
	}
	return sub, closer(ctx, "kafka subscriber", sub.Close), nil
}
