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

// Client represent a MQTT client.
type Client struct {
	*autopaho.ConnectionManager
	route paho.Router
}

// config represent a MQTT client configuration.
type config struct {
	keepAlive         int
	connectRetryDelay time.Duration
	name              string
	retained          bool
	user              string
	password          string
	tls               *tls.Config
}

type Option func(*config)

func EnableRetained() Option {
	return func(c *config) {
		c.retained = true
	}
}

func WithKeepAlive(keepAlive int) Option {
	return func(c *config) {
		c.keepAlive = keepAlive
	}
}

func WithRetry(delay time.Duration) Option {
	return func(c *config) {
		c.connectRetryDelay = delay
	}
}

func WithUser(user string) Option {
	return func(c *config) {
		c.user = user
	}
}

func WithPassword(password string) Option {
	return func(c *config) {
		c.password = password
	}
}

func WithTLS(tls *tls.Config) Option {
	return func(c *config) {
		c.tls = tls
	}
}

func NewClient(address string, clientID string, options ...Option) (*Client, error) {
	if len(address) == 0 {
		return nil, errors.New("address should not be empty")
	}

	l := log.L()
	ctx := context.Background()

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
		name:              clientID,
		retained:          false,
		tls:               nil,
	}

	for _, o := range options {
		o(opts)
	}

	router := paho.NewStandardRouter()
	conf := autopaho.ClientConfig{
		BrokerUrls:       []*url.URL{u},
		TlsCfg:           nil,
		KeepAlive:        uint16(opts.keepAlive),
		ReconnectBackoff: autopaho.NewConstantBackoff(opts.connectRetryDelay),
		ConnectTimeout:   10 * time.Second,
		// autopaho reconnects automatically, errors are only reported.
		OnConnectError: func(err error) {
			l.Error(ctx, "failed to connect to MQTT client", log.Error(err))
		},
		ClientID: clientID,
		Router:   router,
		OnClientError: func(err error) {
			l.Error(ctx, "client disconnected", log.Error(err))
		},
		OnServerDisconnect: func(disconnect *paho.Disconnect) {
			reason := log.String("reason", "Unknown")
			if disconnect.Properties != nil {
				reason = log.String("reason", disconnect.Properties.ReasonString)
			}
			l.Error(ctx, "server disconnected", log.Int("reason code", int(disconnect.ReasonCode)), reason)
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

	c, err := autopaho.NewConnection(ctx, conf)
	if err != nil {
		return nil, err
	}

	// force a connection to mqtt, ctx is not reused as it is read by the connection callbacks.
	connectCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	err = c.AwaitConnection(connectCtx)
	if err != nil {
		// stop the connection manager, it would otherwise keep reconnecting in the background.
		disconnectCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
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
