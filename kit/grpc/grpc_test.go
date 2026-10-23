package grpc

import (
	"context"
	"net"
	"sync/atomic"
	"testing"
	"time"

	pb "github.com/anthonycorbacho/workspace/api/sample/sampleapp/v1"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/status"
)

type panicHealthServer struct {
	healthpb.UnimplementedHealthServer
}

func (panicHealthServer) Check(context.Context, *healthpb.HealthCheckRequest) (*healthpb.HealthCheckResponse, error) {
	panic("boom")
}

func startServer(t *testing.T, register func(*grpc.Server)) *grpc.ClientConn {
	t.Helper()
	srv := NewServer()
	register(srv)
	InitializeMetrics(srv)

	lis, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	go func() { _ = srv.Serve(lis) }()
	t.Cleanup(srv.Stop)

	cc, err := NewClient(lis.Addr().String(), grpc.WithTransportCredentials(insecure.NewCredentials()))
	require.NoError(t, err)
	t.Cleanup(func() { _ = cc.Close() })
	return cc
}

func TestServerAndClient(t *testing.T) {
	cc := startServer(t, func(srv *grpc.Server) {
		healthpb.RegisterHealthServer(srv, health.NewServer())
	})

	resp, err := healthpb.NewHealthClient(cc).Check(context.Background(), &healthpb.HealthCheckRequest{})
	require.NoError(t, err)
	assert.Equal(t, healthpb.HealthCheckResponse_SERVING, resp.GetStatus())

	families, err := prometheus.DefaultGatherer.Gather()
	require.NoError(t, err)
	names := map[string]bool{}
	for _, f := range families {
		names[f.GetName()] = true
	}
	assert.True(t, names["grpc_server_handled_total"])
	assert.True(t, names["grpc_server_handling_seconds"])
	assert.True(t, names["grpc_client_handled_total"])
}

func TestServerRecoverFromPanic(t *testing.T) {
	cc := startServer(t, func(srv *grpc.Server) {
		healthpb.RegisterHealthServer(srv, panicHealthServer{})
	})

	_, err := healthpb.NewHealthClient(cc).Check(context.Background(), &healthpb.HealthCheckRequest{})
	assert.Equal(t, codes.Internal, status.Code(err))
}

// sampleServer calls fetch for every Fetch request, with the attempt number starting at 1.
type sampleServer struct {
	pb.UnimplementedSampleAppServiceServer
	calls atomic.Int32
	fetch func(ctx context.Context, attempt int) error
}

func (s *sampleServer) Fetch(ctx context.Context, req *pb.FetchRequest) (*pb.FetchResponse, error) {
	attempt := int(s.calls.Add(1))
	if s.fetch != nil {
		if err := s.fetch(ctx, attempt); err != nil {
			return nil, err
		}
	}
	return &pb.FetchResponse{Name: req.GetId()}, nil
}

// failTimes returns a fetch function failing with the given code for the first n attempts.
func failTimes(n int, code codes.Code) func(context.Context, int) error {
	return func(_ context.Context, attempt int) error {
		if attempt <= n {
			return status.Error(code, "failing")
		}
		return nil
	}
}

func TestClientRetry(t *testing.T) {
	tests := []struct {
		name         string
		fetch        func(context.Context, int) error
		opts         []grpc.CallOption
		wantCode     codes.Code
		wantAttempts int32
	}{
		{
			name:         "no retry by default",
			fetch:        failTimes(1, codes.Unavailable),
			wantCode:     codes.Unavailable,
			wantAttempts: 1,
		},
		{
			name:         "retry unavailable until success",
			fetch:        failTimes(2, codes.Unavailable),
			opts:         []grpc.CallOption{WithMaxRetries(3)},
			wantCode:     codes.OK,
			wantAttempts: 3,
		},
		{
			name:         "retry resource exhausted until success",
			fetch:        failTimes(1, codes.ResourceExhausted),
			opts:         []grpc.CallOption{WithMaxRetries(3)},
			wantCode:     codes.OK,
			wantAttempts: 2,
		},
		{
			name:         "retries exhausted (initial call + 2 retries)",
			fetch:        failTimes(5, codes.Unavailable),
			opts:         []grpc.CallOption{WithMaxRetries(2)},
			wantCode:     codes.Unavailable,
			wantAttempts: 3,
		},
		{
			name:         "code not retried by default",
			fetch:        failTimes(1, codes.Aborted),
			opts:         []grpc.CallOption{WithMaxRetries(3)},
			wantCode:     codes.Aborted,
			wantAttempts: 1,
		},
		{
			name:         "retry custom code",
			fetch:        failTimes(2, codes.Aborted),
			opts:         []grpc.CallOption{WithMaxRetries(3), WithCodes(codes.Aborted)},
			wantCode:     codes.OK,
			wantAttempts: 3,
		},
		{
			name:         "custom codes replace the default codes",
			fetch:        failTimes(1, codes.Unavailable),
			opts:         []grpc.CallOption{WithMaxRetries(3), WithCodes(codes.Aborted)},
			wantCode:     codes.Unavailable,
			wantAttempts: 1,
		},
		{
			name: "retry after per retry timeout",
			fetch: func(ctx context.Context, attempt int) error {
				if attempt == 1 {
					<-ctx.Done()
					return ctx.Err()
				}
				return nil
			},
			opts:         []grpc.CallOption{WithMaxRetries(3), WithPerRetryTimeout(100 * time.Millisecond)},
			wantCode:     codes.OK,
			wantAttempts: 2,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := &sampleServer{fetch: tt.fetch}
			cc := startServer(t, func(s *grpc.Server) { pb.RegisterSampleAppServiceServer(s, srv) })

			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			resp, err := pb.NewSampleAppServiceClient(cc).Fetch(ctx, &pb.FetchRequest{Id: "abcd"}, tt.opts...)
			assert.Equal(t, tt.wantCode, status.Code(err), "error: %v", err)
			assert.Equal(t, tt.wantAttempts, srv.calls.Load())
			if tt.wantCode == codes.OK {
				assert.Equal(t, "abcd", resp.GetName())
			}
		})
	}
}

func TestValidation(t *testing.T) {
	tests := []struct {
		name         string
		kitClient    bool
		id           string
		wantCode     codes.Code
		wantAttempts int32
	}{
		{name: "client accepts valid request", kitClient: true, id: "abcd", wantCode: codes.OK, wantAttempts: 1},
		{name: "client rejects invalid request", kitClient: true, id: "ab", wantCode: codes.InvalidArgument},
		{name: "server accepts valid request", id: "abcd", wantCode: codes.OK, wantAttempts: 1},
		{name: "server rejects invalid request", id: "ab", wantCode: codes.InvalidArgument},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := &sampleServer{}
			cc := startServer(t, func(s *grpc.Server) { pb.RegisterSampleAppServiceServer(s, srv) })
			if !tt.kitClient {
				// a plain client does not validate the requests, the server does.
				var err error
				cc, err = grpc.NewClient(cc.Target(), grpc.WithTransportCredentials(insecure.NewCredentials()))
				require.NoError(t, err)
				t.Cleanup(func() { _ = cc.Close() })
			}

			_, err := pb.NewSampleAppServiceClient(cc).Fetch(context.Background(), &pb.FetchRequest{Id: tt.id}, WithMaxRetries(3))
			assert.Equal(t, tt.wantCode, status.Code(err), "error: %v", err)
			assert.Equal(t, tt.wantAttempts, srv.calls.Load(), "invalid requests do not reach the handler")
		})
	}
}
