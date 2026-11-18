package kit

import (
	"slices"
	"time"

	"github.com/anthonycorbacho/workspace/kit/log"
	"github.com/rs/cors"
	"google.golang.org/grpc"
)

// options are the configurable options of a Foundation.
type options struct {
	grpcAddr         string
	httpAddr         string
	metricsAddr      string
	probesAddr       string
	grpcServerOpts   []grpc.ServerOption
	gatewayDialOpts  []grpc.DialOption
	cors             *cors.Options
	httpWriteTimeout time.Duration
	httpReadTimeout  time.Duration
	shutdownTimeout  time.Duration
	logger           *log.Logger
}

// Option defines a Foundation option.
type Option func(*options)

// WithGRPCServerOptions defines the gRPC server options.
// Configure the gateway's client credentials with WithGatewayDialOptions when enabling TLS.
func WithGRPCServerOptions(opts ...grpc.ServerOption) Option {
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
func WithHTTPWriteTimeout(timeout time.Duration) Option {
	return func(o *options) {
		o.httpWriteTimeout = timeout
	}
}

// WithHTTPReadTimeout defines the read timeout of the HTTP server.
func WithHTTPReadTimeout(timeout time.Duration) Option {
	return func(o *options) {
		o.httpReadTimeout = timeout
	}
}

// WithShutdownTimeout sets the shared grace period for draining the HTTP, gRPC, probes and metrics servers.
// It defaults to 15 seconds and must be positive. Remaining connections are forcibly closed when it expires.
// The telemetry providers have separate flush timeouts.
func WithShutdownTimeout(timeout time.Duration) Option {
	return func(o *options) {
		o.shutdownTimeout = timeout
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
