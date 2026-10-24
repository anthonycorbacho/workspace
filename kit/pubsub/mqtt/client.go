package mqtt

import (
	"context"
	"crypto/tls"
	"errors"
	"net/url"
	"time"

	"github.com/anthonycorbacho/workspace/kit/log"
	"github.com/eclipse/paho.golang/autopaho"
	"github.com/eclipse/paho.golang/paho"
)

// Client is a MQTT v5 client, shared by a Publisher or a Subscriber, it reconnects automatically.
type Client struct {
	*autopaho.ConnectionManager
	route paho.Router
}

// config represent a MQTT client configuration.
type config struct {
	keepAlive         int
	connectRetryDelay time.Duration
	user              string
	password          string
	tls               *tls.Config
}

// Option configures a Client.
type Option func(*config)

// WithKeepAlive sets the keep alive interval in seconds, 30 by default.
func WithKeepAlive(keepAlive int) Option {
	return func(c *config) {
		c.keepAlive = keepAlive
	}
}

// WithRetry sets the delay between the connection attempts, 5s by default.
func WithRetry(delay time.Duration) Option {
	return func(c *config) {
		c.connectRetryDelay = delay
	}
}

// WithUser sets the user name of the connection.
func WithUser(user string) Option {
	return func(c *config) {
		c.user = user
	}
}

// WithPassword sets the password of the connection, it is only sent with a user (see WithUser).
func WithPassword(password string) Option {
	return func(c *config) {
		c.password = password
	}
}

// WithTLS connects to the broker with TLS.
func WithTLS(tls *tls.Config) Option {
	return func(c *config) {
		c.tls = tls
	}
}

// NewClient creates a MQTT client identified by clientID and connects to the broker at address
// (eg: mqtt://localhost:1883), it waits for the connection until ctx is done or 5s.
//
// The client reconnects automatically until it is disconnected, its connection is not canceled
// with ctx: the values of ctx are kept for the logs of the connection.
func NewClient(ctx context.Context, address string, clientID string, options ...Option) (*Client, error) {
	if len(address) == 0 {
		return nil, errors.New("address should not be empty")
	}

	l := log.L()

	// Validate server address
	u, err := url.ParseRequestURI(address)
	if err != nil {
		return nil, errors.New("address is not valid")
	}
	if len(u.Scheme) == 0 {
		u.Scheme = "tcp"
	}

	if len(clientID) == 0 {
		return nil, errors.New("client ID should not be empty")
	}

	// Init default configuration
	opts := &config{
		keepAlive:         30,
		connectRetryDelay: time.Second * 5,
	}

	for _, o := range options {
		o(opts)
	}

	// the connection lasts until Disconnect, the callbacks log with its context.
	connCtx := context.WithoutCancel(ctx)
	router := paho.NewStandardRouter()
	conf := autopaho.ClientConfig{
		BrokerUrls:       []*url.URL{u},
		TlsCfg:           nil,
		KeepAlive:        uint16(opts.keepAlive),
		ReconnectBackoff: autopaho.NewConstantBackoff(opts.connectRetryDelay),
		ConnectTimeout:   10 * time.Second,
		// autopaho reconnects automatically, errors are only reported.
		OnConnectError: func(err error) {
			l.Error(connCtx, "failed to connect to MQTT client", log.Error(err))
		},
		ClientID: clientID,
		Router:   router,
		OnClientError: func(err error) {
			l.Error(connCtx, "client disconnected", log.Error(err))
		},
		OnServerDisconnect: func(disconnect *paho.Disconnect) {
			reason := log.String("reason", "Unknown")
			if disconnect.Properties != nil {
				reason = log.String("reason", disconnect.Properties.ReasonString)
			}
			l.Error(connCtx, "server disconnected", log.Int("reason code", int(disconnect.ReasonCode)), reason)
		},
	}

	if opts.tls != nil {
		brokerURLs := make([]*url.URL, 0, len(conf.BrokerUrls))
		for _, b := range conf.BrokerUrls {
			b.Scheme = "tls"
			brokerURLs = append(brokerURLs, b)
		}
		conf.BrokerUrls = brokerURLs
		conf.TlsCfg = opts.tls
	}

	if len(opts.user) != 0 {
		conf.ConnectUsername = opts.user
		conf.ConnectPassword = []byte(opts.password)
	}

	// the connection manager runs until its context is done, connCtx is never done: until Disconnect.
	c, err := autopaho.NewConnection(connCtx, conf)
	if err != nil {
		return nil, err
	}

	// force a connection to mqtt.
	connectCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	err = c.AwaitConnection(connectCtx)
	if err != nil {
		// stop the connection manager, it would otherwise keep reconnecting in the background.
		disconnectCtx, cancel := context.WithTimeout(connCtx, 5*time.Second)
		defer cancel()
		if dErr := c.Disconnect(disconnectCtx); dErr != nil {
			l.Warn(disconnectCtx, "disconnecting MQTT client", log.Error(dErr))
		}
		return nil, err
	}

	client := Client{
		ConnectionManager: c,
		route:             router,
	}

	return &client, nil
}
