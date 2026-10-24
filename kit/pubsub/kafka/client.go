package kafka

import (
	"context"
	"crypto/tls"
	"errors"
	"time"

	"github.com/twmb/franz-go/pkg/kgo"
	"github.com/twmb/franz-go/pkg/sasl/plain"
	"github.com/twmb/franz-go/pkg/sasl/scram"
)

// Client is a Kafka client, shared by a Publisher or a Subscriber.
type Client struct {
	client            *kgo.Client
	autoCommitEnabled bool
	regexEnabled      bool
	regexPatterns     map[string]struct{}
}

// config represents a Kafka client configuration.
type config struct {
	clientID          string
	consumerGroup     string
	autoCommitEnabled bool
	tls               *tls.Config
	saslMechanism     string
	saslUsername      string
	saslPassword      string
	regexEnabled      bool
	regexPatterns     []string
	autoTopicCreation bool
}

// Option configures a Client.
type Option func(*config)

// WithClientID sets the client ID.
func WithClientID(clientID string) Option {
	return func(c *config) {
		c.clientID = clientID
	}
}

// WithConsumerGroup sets the consumer group.
func WithConsumerGroup(group string) Option {
	return func(c *config) {
		c.consumerGroup = group
	}
}

// WithAutoCommit enables auto commit.
func WithAutoCommit() Option {
	return func(c *config) {
		c.autoCommitEnabled = true
	}
}

// WithTLS sets the TLS configuration.
func WithTLS(tls *tls.Config) Option {
	return func(c *config) {
		c.tls = tls
	}
}

// WithSASLPlain sets SASL plain authentication.
func WithSASLPlain(username, password string) Option {
	return func(c *config) {
		c.saslMechanism = "PLAIN"
		c.saslUsername = username
		c.saslPassword = password
	}
}

// WithSASLScramSHA256 sets SASL SCRAM-SHA-256 authentication.
func WithSASLScramSHA256(username, password string) Option {
	return func(c *config) {
		c.saslMechanism = "SCRAM-SHA-256"
		c.saslUsername = username
		c.saslPassword = password
	}
}

// WithSASLScramSHA512 sets SASL SCRAM-SHA-512 authentication.
func WithSASLScramSHA512(username, password string) Option {
	return func(c *config) {
		c.saslMechanism = "SCRAM-SHA-512"
		c.saslUsername = username
		c.saslPassword = password
	}
}

// WithRegex consumes the topics matching the regex patterns, a Subscriber must subscribe to
// each pattern (see Subscriber.SubscribeWithAck) before the records are polled.
func WithRegex(patterns []string) Option {
	return func(c *config) {
		c.regexEnabled = true
		c.regexPatterns = patterns
	}
}

// WithAutoTopicCreation allows the broker to create topics that do not exist yet
// when producing to them. The broker must also have topic auto creation enabled.
func WithAutoTopicCreation() Option {
	return func(c *config) {
		c.autoTopicCreation = true
	}
}

// NewClient creates a Kafka client connecting to brokers, it connects lazily.
func NewClient(brokers []string, options ...Option) (*Client, error) {
	if len(brokers) == 0 {
		return nil, errors.New("brokers should not be empty")
	}

	// Init default configuration
	opts := &config{
		clientID:          "kafka-client",
		autoCommitEnabled: false,
		regexEnabled:      false,
	}

	for _, o := range options {
		o(opts)
	}

	// Build kafka options
	kafkaOpts := []kgo.Opt{
		kgo.SeedBrokers(brokers...),
		kgo.ClientID(opts.clientID),
		kgo.ConnIdleTimeout(90 * time.Second),
		kgo.RequestTimeoutOverhead(10 * time.Second),
		kgo.ProducerBatchMaxBytes(1000000), // 1MB
		kgo.ProducerLinger(10 * time.Millisecond),
	}

	// Add TLS if configured
	if opts.tls != nil {
		kafkaOpts = append(kafkaOpts, kgo.DialTLSConfig(opts.tls))
	}

	// Add SASL if configured
	if opts.saslMechanism != "" {
		switch opts.saslMechanism {
		case "PLAIN":
			kafkaOpts = append(kafkaOpts, kgo.SASL(plain.Auth{
				User: opts.saslUsername,
				Pass: opts.saslPassword,
			}.AsMechanism()))
		case "SCRAM-SHA-256":
			kafkaOpts = append(kafkaOpts, kgo.SASL(scram.Auth{
				User: opts.saslUsername,
				Pass: opts.saslPassword,
			}.AsSha256Mechanism()))
		case "SCRAM-SHA-512":
			kafkaOpts = append(kafkaOpts, kgo.SASL(scram.Auth{
				User: opts.saslUsername,
				Pass: opts.saslPassword,
			}.AsSha512Mechanism()))
		}
	}

	// Add consumer group if configured
	if opts.consumerGroup != "" {
		kafkaOpts = append(kafkaOpts, kgo.ConsumerGroup(opts.consumerGroup))
		kafkaOpts = append(kafkaOpts, kgo.SessionTimeout(30*time.Second))
		kafkaOpts = append(kafkaOpts, kgo.RebalanceTimeout(60*time.Second))

		if opts.autoCommitEnabled {
			kafkaOpts = append(kafkaOpts, kgo.AutoCommitInterval(3*time.Second))
			kafkaOpts = append(kafkaOpts, kgo.AutoCommitMarks())
		} else {
			kafkaOpts = append(kafkaOpts, kgo.DisableAutoCommit())
		}
	}

	if opts.autoTopicCreation {
		kafkaOpts = append(kafkaOpts, kgo.AllowAutoTopicCreation())
	}

	// Add regex if configured
	var regexPatterns map[string]struct{}
	if opts.regexEnabled {
		kafkaOpts = append(kafkaOpts, kgo.ConsumeRegex())
		kafkaOpts = append(kafkaOpts, kgo.ConsumeTopics(opts.regexPatterns...))

		regexPatterns = make(map[string]struct{})
		for _, pattern := range opts.regexPatterns {
			regexPatterns[pattern] = struct{}{}
		}
	}

	client, err := kgo.NewClient(kafkaOpts...)
	if err != nil {
		return nil, err
	}

	return &Client{
		client:            client,
		autoCommitEnabled: opts.autoCommitEnabled,
		regexEnabled:      opts.regexEnabled,
		regexPatterns:     regexPatterns,
	}, nil
}

// Close closes the Kafka client.
func (c *Client) Close() {
	c.client.Close()
}

// Ping returns whether any broker is reachable.
func (c *Client) Ping(ctx context.Context) error {
	return c.client.Ping(ctx)
}
