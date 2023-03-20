package kit

import (
	"time"

	"github.com/anthonycorbacho/workspace/kit/log"
	"github.com/rs/cors"
	"google.golang.org/grpc"
)

// FoundationOptions provides a set of configurable options for Foundation.
type FoundationOptions struct {
	grpcAddr         string
	httpAddr         string
	grpcServerOpts   []grpc.ServerOption
	corsOpts         cors.Options
	enableCors       bool
	httpWriteTimeout time.Duration
	httpReadTimeout  time.Duration
	logger           *log.Logger
}

// Option defines a Foundation option.
type Option func(*FoundationOptions)

// WithGrpcServerOptions defines GRPC server options.
func WithGrpcServerOptions(opts ...grpc.ServerOption) Option {
	return func(fo *FoundationOptions) {
		fo.grpcServerOpts = opts
	}
}

// EnableCors will add cors support to the http server.
func EnableCors() Option {
	return func(fo *FoundationOptions) {
		fo.enableCors = true
	}
}

// WithCorsOptions defines http server cors options.
func WithCorsOptions(opts cors.Options) Option {
	return func(fo *FoundationOptions) {
		fo.corsOpts = opts
	}
}

// WithGrpcAddr defines a GRPC server host and port.
func WithGrpcAddr(addr string) Option {
	return func(fo *FoundationOptions) {
		fo.grpcAddr = addr
	}
}

// WithHTTPAddr defines a HTTP server host and port.
func WithHTTPAddr(addr string) Option {
	return func(fo *FoundationOptions) {
		fo.httpAddr = addr
	}
}

// WithHTTPWriteTimeout defines write timeout for the HTTP server.
func WithHTTPWriteTimeout(timeout time.Duration) Option {
	return func(fo *FoundationOptions) {
		fo.httpWriteTimeout = timeout
	}
}

// WithHTTPReadTimeout defines read timeout for the HTTP server.
func WithHTTPReadTimeout(timeout time.Duration) Option {
	return func(fo *FoundationOptions) {
		fo.httpReadTimeout = timeout
	}
}

func WithLogger(logger *log.Logger) Option {
	return func(fo *FoundationOptions) {
		fo.logger = logger
	}
}
