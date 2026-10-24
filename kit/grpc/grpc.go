package grpc

import (
	"sync"
	"time"

	"github.com/anthonycorbacho/workspace/kit/log"
	grpcprometheus "github.com/grpc-ecosystem/go-grpc-middleware/providers/prometheus"
	grpcrecovery "github.com/grpc-ecosystem/go-grpc-middleware/v2/interceptors/recovery"
	grpcretry "github.com/grpc-ecosystem/go-grpc-middleware/v2/interceptors/retry"
	"github.com/prometheus/client_golang/prometheus"
	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"go.opentelemetry.io/otel/metric/noop"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
)

var (
	metricsOnce   sync.Once
	serverMetrics *grpcprometheus.ServerMetrics
	clientMetrics *grpcprometheus.ClientMetrics
)

// metrics returns the gRPC server and client prometheus metrics,
// registered once in the default prometheus registry.
func metrics() (*grpcprometheus.ServerMetrics, *grpcprometheus.ClientMetrics) {
	metricsOnce.Do(func() {
		serverMetrics = grpcprometheus.NewServerMetrics(grpcprometheus.WithServerHandlingTimeHistogram())
		clientMetrics = grpcprometheus.NewClientMetrics()
		prometheus.MustRegister(serverMetrics, clientMetrics)
	})
	return serverMetrics, clientMetrics
}

// InitializeMetrics initializes all the server metrics with zero values for all the
// registered gRPC methods. It must be called after all the services have been registered to the server.
func InitializeMetrics(srv *grpc.Server) {
	srvMetrics, _ := metrics()
	srvMetrics.InitializeMetrics(srv)
}

// otelgrpcOptions are the options of the tracing stats handlers, their metrics are disabled since
// the prometheus interceptors already record them.
func otelgrpcOptions() []otelgrpc.Option {
	return []otelgrpc.Option{otelgrpc.WithMeterProvider(noop.NewMeterProvider())}
}

// NewServer creates a gRPC server that will be by default
// recover from panic and setup for observability: traces (continuing the trace of the client, see
// telemetry.NewTracer), prometheus metrics and logs.
//
// The errors returned to the client are coded errors (see kit/errors): the handlers errors are converted
// with errors.FromError (a plain error becomes an Internal error, logged but never sent to the client),
// the invalid requests are rejected with an InvalidRequest error listing the violations.
//
// The server errors and the recovered panics are logged with l, the global logger is used if l is nil.
func NewServer(l *log.Logger, opts ...grpc.ServerOption) *grpc.Server {
	if l == nil {
		l = log.L()
	}
	// Create a default server opts and set our default chain of interceptor
	// if user decide to pass a custom interceptor via `grpc.ChainXXXInterceptor` or grpc.XXXInterceptor,
	// it should be added at the end of the call chain since
	// interpreter call chain is from left to right.
	// The error interceptor is after the metrics one, so the metrics record the status sent to the client.
	srvMetrics, _ := metrics()
	recovery := grpcrecovery.WithRecoveryHandlerContext(recoverFrom(l))
	serverOpts := []grpc.ServerOption{
		grpc.StatsHandler(otelgrpc.NewServerHandler(otelgrpcOptions()...)),
		grpc.ChainStreamInterceptor(
			grpcrecovery.StreamServerInterceptor(recovery),
			srvMetrics.StreamServerInterceptor(),
			streamServerErrorInterceptor(l),
			streamServerValidatorInterceptor(),
		),
		grpc.ChainUnaryInterceptor(
			grpcrecovery.UnaryServerInterceptor(recovery),
			srvMetrics.UnaryServerInterceptor(),
			unaryServerErrorInterceptor(l),
			unaryServerValidatorInterceptor(),
		),
	}

	serverOpts = append(serverOpts, opts...)
	srv := grpc.NewServer(serverOpts...)
	return srv
}

// NewClient create a new gRPC client setup for observability (the trace is propagated to the server) and retry.
//
// By default, the reties *are disabled*, preventing accidental use of retries. You can easily
// override the number of retries (setting them to more than 0) with a `grpc.ClientOption`, e.g.:
//
//	myclient.Ping(ctx, goodPing, grpckit.WithMaxRetries(5))
//
// Other default options are: retry on `ResourceExhausted` and `Unavailable` gRPC codes, use a 50ms
// linear backoff with 10% jitter.
//
// See: https://pkg.go.dev/github.com/grpc-ecosystem/go-grpc-middleware/v2/interceptors/retry
func NewClient(addr string, opts ...grpc.DialOption) (*grpc.ClientConn, error) {
	// Create a default dial opts and set our default chain of interceptor
	// if user decide to pass a custom interceptor via `grpc.WithChainXXXInterceptor` or grpc.XXXInterceptor,
	// it should be added at the end of the call chain since
	// interpreter call chain is from left to right.
	// The errors received from the server are converted to coded errors (see kit/errors) once retried.
	_, clMetrics := metrics()
	dialOps := []grpc.DialOption{
		grpc.WithStatsHandler(otelgrpc.NewClientHandler(otelgrpcOptions()...)),
		grpc.WithChainUnaryInterceptor(
			unaryClientErrorInterceptor(),
			grpcretry.UnaryClientInterceptor(),
			clMetrics.UnaryClientInterceptor(),
			unaryClientValidatorInterceptor(),
		),
		grpc.WithChainStreamInterceptor(
			streamClientErrorInterceptor(),
			grpcretry.StreamClientInterceptor(),
			clMetrics.StreamClientInterceptor(),
		),
	}
	dialOps = append(dialOps, opts...)
	return grpc.NewClient(addr, dialOps...)
}

// WithMaxRetries sets the maximum number of retries on this call, or this interceptor.
// The call is attempted at most maxRetries+1 times (the initial call and its retries).
func WithMaxRetries(maxRetries uint) grpcretry.CallOption {
	// grpcretry.WithMax is the maximum number of attempts, the initial call included.
	return grpcretry.WithMax(maxRetries + 1)
}

// WithPerRetryTimeout sets the RPC timeout per call (including initial call) on this call, or this interceptor.
//
// The context.Deadline of the call takes precedence and sets the maximum time the whole invocation
// will take, but WithPerRetryTimeout can be used to limit the RPC time per each call.
//
// For example, with context.Deadline = now + 10s, and WithPerRetryTimeout(3 * time.Seconds), each
// of the retry calls (including the initial one) will have a deadline of now + 3s.
//
// A value of 0 disables the timeout overrides completely and returns to each retry call using the
// parent `context.Deadline`.
//
// Note that when this is enabled, any DeadlineExceeded errors that are propagated up will be retried.
func WithPerRetryTimeout(timeout time.Duration) grpcretry.CallOption {
	return grpcretry.WithPerRetryTimeout(timeout)
}

// WithCodes sets which codes should be retried.
//
// Please *use with care*, as you may be retrying non-idempotent calls.
//
// You cannot automatically retry on Cancelled and Deadline, please use `WithPerRetryTimeout` for these.
func WithCodes(retryCodes ...codes.Code) grpcretry.CallOption {
	return grpcretry.WithCodes(retryCodes...)
}
