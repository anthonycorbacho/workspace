package grpc

import (
	"context"
	stderrors "errors"
	"io"
	"testing"
	"time"

	pb "github.com/anthonycorbacho/workspace/api/sample/sampleapp/v1"
	"github.com/anthonycorbacho/workspace/kit/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
)

func TestServerErrors(t *testing.T) {
	tests := []struct {
		name        string
		fetch       func(ctx context.Context, attempt int) error
		ctxTimeout  time.Duration
		wantCode    string
		wantKind    errors.Kind
		wantMessage string
		wantDetails map[string]any
	}{
		{
			name:        "coded error",
			fetch:       func(context.Context, int) error { return errors.SampleUserNotFound("abcd") },
			wantCode:    errors.SampleUserNotFoundCode,
			wantKind:    errors.KindNotFound,
			wantMessage: "user abcd not found",
			wantDetails: map[string]any{"id": "abcd"},
		},
		{
			name: "wrapped coded error with cause",
			fetch: func(context.Context, int) error {
				return errors.Wrap(errors.SampleUserNotFound("abcd").WithCause(stderrors.New("sql: no rows")), "fetch")
			},
			wantCode:    errors.SampleUserNotFoundCode,
			wantKind:    errors.KindNotFound,
			wantMessage: "user abcd not found",
			wantDetails: map[string]any{"id": "abcd"},
		},
		{
			name:        "plain error is internal and its message is not sent",
			fetch:       func(context.Context, int) error { return stderrors.New("dial tcp 10.0.0.1:5432: password=secret") },
			wantCode:    errors.InternalCode,
			wantKind:    errors.KindInternal,
			wantMessage: "internal error",
			wantDetails: map[string]any{},
		},
		{
			name: "context deadline",
			fetch: func(ctx context.Context, _ int) error {
				<-ctx.Done()
				return ctx.Err()
			},
			ctxTimeout:  50 * time.Millisecond,
			wantCode:    errors.UnclassifiedCode,
			wantKind:    errors.KindDeadlineExceeded,
			wantMessage: context.DeadlineExceeded.Error(),
			wantDetails: map[string]any{},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := &sampleServer{fetch: tt.fetch}
			cc := startServer(t, func(s *grpc.Server) { pb.RegisterSampleAppServiceServer(s, srv) })

			ctx := context.Background()
			if tt.ctxTimeout > 0 {
				var cancel context.CancelFunc
				ctx, cancel = context.WithTimeout(ctx, tt.ctxTimeout)
				defer cancel()
			}
			_, err := pb.NewSampleAppServiceClient(cc).Fetch(ctx, &pb.FetchRequest{Id: "abcd"})
			require.Error(t, err)

			var coded *errors.Error
			require.True(t, errors.As(err, &coded), "the kit client returns coded errors: %v", err)
			assert.Equal(t, tt.wantCode, coded.Code)
			assert.Equal(t, tt.wantKind, coded.Kind)
			assert.Equal(t, tt.wantMessage, coded.Message)
			assert.Equal(t, tt.wantDetails, coded.Details)
			assert.NotContains(t, err.Error(), "secret", "server internal information must not reach the client")
		})
	}
}

func TestServerPanicIsInternal(t *testing.T) {
	cc := startServer(t, func(srv *grpc.Server) {
		healthpb.RegisterHealthServer(srv, panicHealthServer{})
	})

	_, err := healthpb.NewHealthClient(cc).Check(context.Background(), &healthpb.HealthCheckRequest{})
	assert.ErrorIs(t, err, errors.Internal())
	assert.NotContains(t, err.Error(), "boom", "the panic value must not reach the client")
}

func TestValidationErrors(t *testing.T) {
	tests := []struct {
		name      string
		kitClient bool
	}{
		{name: "rejected by the kit client", kitClient: true},
		{name: "rejected by the server", kitClient: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := &sampleServer{}
			cc := startServer(t, func(s *grpc.Server) { pb.RegisterSampleAppServiceServer(s, srv) })
			if !tt.kitClient {
				var err error
				cc, err = grpc.NewClient(cc.Target(), grpc.WithTransportCredentials(insecure.NewCredentials()))
				require.NoError(t, err)
				t.Cleanup(func() { _ = cc.Close() })
			}

			_, err := pb.NewSampleAppServiceClient(cc).Fetch(context.Background(), &pb.FetchRequest{Id: "ab"})

			coded := FromError(err)
			assert.Equal(t, errors.InvalidRequestCode, coded.Code)
			assert.Equal(t, errors.KindInvalidArgument, coded.Kind)
			assert.Equal(t, []any{
				map[string]any{"field": "id", "message": "must be at least 4 characters"},
			}, coded.Details["violations"])
			assert.Zero(t, srv.calls.Load(), "invalid requests do not reach the handler")
		})
	}
}

func TestCodedErrorPropagatesAcrossServices(t *testing.T) {
	// backend returns a coded error.
	backend := &sampleServer{fetch: func(context.Context, int) error { return errors.SampleUserNotFound("abcd") }}
	backendConn := startServer(t, func(s *grpc.Server) { pb.RegisterSampleAppServiceServer(s, backend) })

	// frontend returns the backend error as is.
	frontend := &sampleServer{fetch: func(ctx context.Context, _ int) error {
		_, err := pb.NewSampleAppServiceClient(backendConn).Fetch(ctx, &pb.FetchRequest{Id: "abcd"})
		return err
	}}
	frontendConn := startServer(t, func(s *grpc.Server) { pb.RegisterSampleAppServiceServer(s, frontend) })

	_, err := pb.NewSampleAppServiceClient(frontendConn).Fetch(context.Background(), &pb.FetchRequest{Id: "abcd"})
	assert.ErrorIs(t, err, errors.SampleUserNotFound(""), "the backend error code reaches the end client")
	assert.Equal(t, map[string]any{"id": "abcd"}, FromError(err).Details)
}

// watchHealthServer fails the Watch stream after sending one message.
type watchHealthServer struct {
	healthpb.UnimplementedHealthServer
	err error
}

func (s watchHealthServer) Watch(_ *healthpb.HealthCheckRequest, stream grpc.ServerStreamingServer[healthpb.HealthCheckResponse]) error {
	if err := stream.Send(&healthpb.HealthCheckResponse{Status: healthpb.HealthCheckResponse_SERVING}); err != nil {
		return err
	}
	return s.err
}

func TestStreamErrors(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		wantCode string
		wantEOF  bool
	}{
		{name: "coded error", err: errors.SampleUserNotFound("abcd"), wantCode: errors.SampleUserNotFoundCode},
		{name: "plain error is internal", err: stderrors.New("secret"), wantCode: errors.InternalCode},
		{name: "end of stream", err: nil, wantEOF: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cc := startServer(t, func(s *grpc.Server) { healthpb.RegisterHealthServer(s, watchHealthServer{err: tt.err}) })

			stream, err := healthpb.NewHealthClient(cc).Watch(context.Background(), &healthpb.HealthCheckRequest{})
			require.NoError(t, err)
			_, err = stream.Recv()
			require.NoError(t, err)

			_, err = stream.Recv()
			if tt.wantEOF {
				assert.ErrorIs(t, err, io.EOF, "io.EOF is kept to mark the end of the stream")
				return
			}
			assert.Equal(t, tt.wantCode, FromError(err).Code)
			assert.NotContains(t, err.Error(), "secret")
		})
	}
}
