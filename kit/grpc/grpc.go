package grpc

import (
	"context"
	"sync"
	"time"

	"buf.build/go/protovalidate"
	"github.com/anthonycorbacho/workspace/kit/log"
	grpcprometheus "github.com/grpc-ecosystem/go-grpc-middleware/providers/prometheus"
	grpcvalidator "github.com/grpc-ecosystem/go-grpc-middleware/v2/interceptors/protovalidate"
	grpcrecovery "github.com/grpc-ecosystem/go-grpc-middleware/v2/interceptors/recovery"
	grpcretry "github.com/grpc-ecosystem/go-grpc-middleware/v2/interceptors/retry"
	"github.com/prometheus/client_golang/prometheus"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
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

// NewServer creates a gRPC server that will be by default
// recover from panic and setup for observability.
func NewServer(opts ...grpc.ServerOption) *grpc.Server {
	// Create a default server opts and set our default chain of interceptor
	// if user decide to pass a custom interceptor via `grpc.ChainXXXInterceptor` or grpc.XXXInterceptor,
	// it should be added at the end of the call chain since
	// interpreter call chain is from left to right.
	srvMetrics, _ := metrics()
	serverOpts := []grpc.ServerOption{
		grpc.ChainStreamInterceptor(
			grpcrecovery.StreamServerInterceptor(grpcrecovery.WithRecoveryHandlerContext(recoverFrom(log.L()))),
			srvMetrics.StreamServerInterceptor(),
			grpcvalidator.StreamServerInterceptor(protovalidate.GlobalValidator),
		),
		grpc.ChainUnaryInterceptor(
			grpcrecovery.UnaryServerInterceptor(grpcrecovery.WithRecoveryHandlerContext(recoverFrom(log.L()))),
			srvMetrics.UnaryServerInterceptor(),
			grpcvalidator.UnaryServerInterceptor(protovalidate.GlobalValidator),
		),
	}

	serverOpts = append(serverOpts, opts...)
	srv := grpc.NewServer(serverOpts...)
	return srv
}

// NewClient create a new gRPC client setup for observability and retry.
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
	_, clMetrics := metrics()
	dialOps := []grpc.DialOption{
		grpc.WithChainUnaryInterceptor(
			grpcretry.UnaryClientInterceptor(),
			clMetrics.UnaryClientInterceptor(),
			unaryClientValidatorInterceptor(),
		),
		grpc.WithChainStreamInterceptor(
			grpcretry.StreamClientInterceptor(),
			clMetrics.StreamClientInterceptor(),
		),
	}
	dialOps = append(dialOps, opts...)
	return grpc.NewClient(addr, dialOps...)
}

// WithMaxRetries sets the maximum number of retries on this call, or this interceptor.
func WithMaxRetries(maxRetries uint) grpcretry.CallOption {
	return grpcretry.WithMax(maxRetries)
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

func recoverFrom(l *log.Logger) grpcrecovery.RecoveryHandlerFuncContext {
	return func(ctx context.Context, p any) error {
		l.Error(ctx, "grpc recover panic", log.Any("panic", p))
		return status.Errorf(codes.Internal, "%v", p)
	}
}

// unaryClientValidatorInterceptor validates the outgoing requests using protovalidate rules
// so invalid requests are rejected before reaching the server.
func unaryClientValidatorInterceptor() grpc.UnaryClientInterceptor {
	return func(ctx context.Context, method string, req, reply any, cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error {
		if msg, ok := req.(proto.Message); ok {
			if err := protovalidate.Validate(msg); err != nil {
				return status.Error(codes.InvalidArgument, err.Error())
			}
		}
		return invoker(ctx, method, req, reply, cc, opts...)
	}
}
