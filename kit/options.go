package kit

import (
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
	cors             *cors.Options
	httpWriteTimeout time.Duration
	httpReadTimeout  time.Duration
	logger           *log.Logger
}

// Option defines a Foundation option.
type Option func(*options)

// WithGRPCServerOptions defines the gRPC server options.
func WithGRPCServerOptions(opts ...grpc.ServerOption) Option {
	return func(o *options) {
		o.grpcServerOpts = opts
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
