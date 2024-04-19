package grpc

import (
	"context"
	"time"

	"github.com/anthonycorbacho/workspace/kit/log"
	grpcrecovery "github.com/grpc-ecosystem/go-grpc-middleware/recovery"
	grpcretry "github.com/grpc-ecosystem/go-grpc-middleware/retry"
	grpcvalidator "github.com/grpc-ecosystem/go-grpc-middleware/validator"
	grpcprometheus "github.com/grpc-ecosystem/go-grpc-prometheus"
	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// NewServer creates a gRPC server that will be by default
// recover from panic and setup for observability.
func NewServer(opts ...grpc.ServerOption) *grpc.Server {
	// Create a default server opts and set our default chain of interceptor
	// if user decide to pass a custom interceptor via `grpc.ChainXXXInterceptor` or grpc.XXXInterceptor,
	// it should be added at the end of the call chain since
	// interpreter call chain is from left to right.
	serverOpts := []grpc.ServerOption{
		grpc.ChainStreamInterceptor(
			otelgrpc.StreamServerInterceptor(),
			grpcrecovery.StreamServerInterceptor(grpcrecovery.WithRecoveryHandlerContext(recoverFrom(log.L()))),
			grpcprometheus.StreamServerInterceptor,
			grpcvalidator.StreamServerInterceptor(),
		),
		grpc.ChainUnaryInterceptor(
			otelgrpc.UnaryServerInterceptor(),
			grpcrecovery.UnaryServerInterceptor(grpcrecovery.WithRecoveryHandlerContext(recoverFrom(log.L()))),
			grpcprometheus.UnaryServerInterceptor,
			grpcvalidator.UnaryServerInterceptor(),
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
// See: https://pkg.go.dev/github.com/grpc-ecosystem/go-grpc-middleware/retry
func NewClient(addr string, opts ...grpc.DialOption) (*grpc.ClientConn, error) {
	// Create a default dial opts and set our default chain of interceptor
	// if user decide to pass a custom interceptor via `grpc.WithChainXXXInterceptor` or grpc.XXXInterceptor,
	// it should be added at the end of the call chain since
	// interpreter call chain is from left to right.
	dialOps := []grpc.DialOption{
		grpc.WithChainUnaryInterceptor(
			otelgrpc.UnaryClientInterceptor(),
			grpcretry.UnaryClientInterceptor(),
			grpcprometheus.UnaryClientInterceptor,
			grpcvalidator.UnaryClientInterceptor(),
		),
		grpc.WithChainStreamInterceptor(
			otelgrpc.StreamClientInterceptor(),
			grpcretry.StreamClientInterceptor(),
			grpcprometheus.StreamClientInterceptor,
		),
	}
	dialOps = append(dialOps, opts...)
	return grpc.Dial(addr, dialOps...)
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
	return func(ctx context.Context, p interface{}) error {
		l.Error(ctx, "grpc recover panic", log.Any("panic", p))
		return status.Errorf(codes.Internal, "%v", p)
	}
}
