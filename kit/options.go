package kit

import (
	"net/http"
	"slices"
	"time"

	"github.com/anthonycorbacho/workspace/kit/log"
	"github.com/rs/cors"
	"google.golang.org/grpc"
)

// options are the configurable options of a Foundation.
type options struct {
	grpcAddr              string
	httpAddr              string
	metricsAddr           string
	probesAddr            string
	grpcServerOpts        []grpc.ServerOption
	gatewayDialOpts       []grpc.DialOption
	gatewayAllowedHeaders map[string]struct{}
	cors                  *cors.Options
	httpWriteTimeout      time.Duration
	httpReadTimeout       time.Duration
	httpReadHeaderTimeout time.Duration
	httpIdleTimeout       time.Duration
	httpMaxHeaderBytes    int
	shutdownTimeout       time.Duration
	shutdownDelay         time.Duration
	logger                *log.Logger
}

// Option defines a Foundation option.
type Option func(*options)

// WithGRPCServerOptions defines the gRPC server options.
// Configure the gateway's client credentials with WithGatewayDialOptions when enabling TLS.
func WithGRPCServerOptions(opts ...grpc.ServerOption) Option {
	opts = slices.Clone(opts)
	return func(o *options) {
		o.grpcServerOpts = opts
	}
}

// WithGatewayDialOptions sets the gRPC dial options used by the HTTP gateway to connect to WithGRPCAddr.
// It replaces the default insecure credentials option: include grpc.WithTransportCredentials or
// grpc.WithCredentialsBundle with credentials matching the gRPC server's configuration.
func WithGatewayDialOptions(opts ...grpc.DialOption) Option {
	opts = slices.Clone(opts)
	return func(o *options) {
		o.gatewayDialOpts = opts
	}
}

// WithGatewayAllowedHeaders restricts the extra HTTP headers forwarded as gRPC metadata by the
// grpc-gateway to the given allowlist (in addition to runtime.DefaultHeaderMatcher).
// Passing no headers disables forwarding of extra headers such as X-*.
// When this option is not set, all X-* headers are forwarded by default.
func WithGatewayAllowedHeaders(headers ...string) Option {
	allowed := make(map[string]struct{}, len(headers))
	for _, h := range headers {
		if h == "" {
			continue
		}
		allowed[http.CanonicalHeaderKey(h)] = struct{}{}
	}
	return func(o *options) {
		o.gatewayAllowedHeaders = allowed
	}
}

// WithCORS enables CORS on the HTTP server with the given options.
func WithCORS(opts cors.Options) Option {
	return func(o *options) {
		o.cors = &opts
	}
}

// WithGRPCAddr defines the gRPC server host and port.
func WithGRPCAddr(addr string) Option {
	return func(o *options) {
		o.grpcAddr = addr
	}
}

// WithHTTPAddr defines the HTTP server host and port.
func WithHTTPAddr(addr string) Option {
	return func(o *options) {
		o.httpAddr = addr
	}
}

// WithHTTPWriteTimeout defines the write timeout of the HTTP server.
// It defaults to 15 seconds. Set to 0 to disable the write timeout (eg: for SSE or streaming responses).
func WithHTTPWriteTimeout(timeout time.Duration) Option {
	return func(o *options) {
		o.httpWriteTimeout = timeout
	}
}

// WithHTTPReadTimeout defines the read timeout of the HTTP server.
// It defaults to 15 seconds.
func WithHTTPReadTimeout(timeout time.Duration) Option {
	return func(o *options) {
		o.httpReadTimeout = timeout
	}
}

// WithHTTPReadHeaderTimeout defines the amount of time allowed to read HTTP request headers.
// It defaults to 5 seconds.
func WithHTTPReadHeaderTimeout(timeout time.Duration) Option {
	return func(o *options) {
		o.httpReadHeaderTimeout = timeout
	}
}

// WithHTTPIdleTimeout defines the maximum amount of time to wait for the next request when keep-alives are enabled.
// It defaults to 60 seconds.
func WithHTTPIdleTimeout(timeout time.Duration) Option {
	return func(o *options) {
		o.httpIdleTimeout = timeout
	}
}

// WithHTTPMaxHeaderBytes defines the maximum number of bytes the HTTP server will read parsing request headers.
// It defaults to 1 MiB.
func WithHTTPMaxHeaderBytes(maxBytes int) Option {
	return func(o *options) {
		o.httpMaxHeaderBytes = maxBytes
	}
}

// WithShutdownTimeout sets the shared grace period for draining the HTTP, gRPC, probes and metrics servers
// and running registered shutdown hooks. It defaults to 15 seconds and must be positive.
// Remaining connections are forcibly closed when it expires.
// The telemetry providers have separate flush timeouts.
func WithShutdownTimeout(timeout time.Duration) Option {
	return func(o *options) {
		o.shutdownTimeout = timeout
	}
}

// WithShutdownDelay sets the pre-drain delay between marking /readyz unready and stopping the public
// listeners on graceful shutdown (also configurable via FOUNDATION_SHUTDOWN_DELAY).
// It defaults to 0. In Kubernetes, setting this (eg: 5s) allows EndpointSlice/kube-proxy propagation
// to stop routing traffic to the pod before the servers close their listeners.
func WithShutdownDelay(delay time.Duration) Option {
	return func(o *options) {
		o.shutdownDelay = delay
	}
}

// WithLogger defines the logger of the foundation and of its servers, a nil logger is ignored.
func WithLogger(logger *log.Logger) Option {
	return func(o *options) {
		if logger != nil {
			o.logger = logger
		}
	}
}

// WithMetricsAddr defines the address of the HTTP server exposing the prometheus /metrics endpoint.
func WithMetricsAddr(addr string) Option {
	return func(o *options) {
		o.metricsAddr = addr
	}
}

// WithProbesAddr defines the address of the HTTP server exposing the health probes and profiling endpoints.
func WithProbesAddr(addr string) Option {
	return func(o *options) {
		o.probesAddr = addr
	}
}
