package grpc

import (
	"bytes"
	"context"
	"errors"
	"net"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	pb "github.com/anthonycorbacho/workspace/api/sample/sampleapp/v1"
	"github.com/anthonycorbacho/workspace/kit/log"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"
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

type errorHealthServer struct {
	healthpb.UnimplementedHealthServer
}

func (errorHealthServer) Check(context.Context, *healthpb.HealthCheckRequest) (*healthpb.HealthCheckResponse, error) {
	return nil, errors.New("connection refused")
}

func startServer(t *testing.T, register func(*grpc.Server)) *grpc.ClientConn {
	t.Helper()
	return startServerWithLogger(t, nil, register)
}

func startServerWithLogger(t *testing.T, l *log.Logger, register func(*grpc.Server)) *grpc.ClientConn {
	t.Helper()
	srv := NewServer(l)
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

// syncBuffer is a concurrent safe buffer used as log output.
type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

func TestServerLogger(t *testing.T) {
	tests := []struct {
		name    string
		handler healthpb.HealthServer
		wantLog string
	}{
		{name: "panic", handler: panicHealthServer{}, wantLog: "grpc recover panic"},
		{name: "internal error", handler: errorHealthServer{}, wantLog: "grpc request failed"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out := &syncBuffer{}
			l, err := log.New(log.WithOutput(out))
			require.NoError(t, err)
			cc := startServerWithLogger(t, l, func(srv *grpc.Server) {
				healthpb.RegisterHealthServer(srv, tt.handler)
			})

			_, err = healthpb.NewHealthClient(cc).Check(context.Background(), &healthpb.HealthCheckRequest{})
			assert.Equal(t, codes.Internal, status.Code(err))
			assert.Contains(t, out.String(), tt.wantLog)
		})
	}
}

// spanRecorder records the ended spans.
type spanRecorder struct {
	mu    sync.Mutex
	spans []sdktrace.ReadOnlySpan
}

func (r *spanRecorder) OnStart(context.Context, sdktrace.ReadWriteSpan) {}
func (r *spanRecorder) OnEnd(s sdktrace.ReadOnlySpan) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.spans = append(r.spans, s)
}
func (r *spanRecorder) Shutdown(context.Context) error   { return nil }
func (r *spanRecorder) ForceFlush(context.Context) error { return nil }

func (r *spanRecorder) byKind() map[trace.SpanKind]sdktrace.ReadOnlySpan {
	r.mu.Lock()
	defer r.mu.Unlock()
	spans := map[trace.SpanKind]sdktrace.ReadOnlySpan{}
	for _, s := range r.spans {
		spans[s.SpanKind()] = s
	}
	return spans
}

func TestTracePropagation(t *testing.T) {
	spans := &spanRecorder{}
	tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(spans))
	prevTP, prevProp := otel.GetTracerProvider(), otel.GetTextMapPropagator()
	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.TraceContext{})
	t.Cleanup(func() {
		otel.SetTracerProvider(prevTP)
		otel.SetTextMapPropagator(prevProp)
	})

	cc := startServer(t, func(srv *grpc.Server) {
		healthpb.RegisterHealthServer(srv, health.NewServer())
	})
	_, err := healthpb.NewHealthClient(cc).Check(context.Background(), &healthpb.HealthCheckRequest{})
	require.NoError(t, err)

	// the server span ends after the response is sent.
	require.Eventually(t, func() bool { return len(spans.byKind()) == 2 }, time.Second, 10*time.Millisecond)
	client, server := spans.byKind()[trace.SpanKindClient], spans.byKind()[trace.SpanKindServer]
	require.NotNil(t, client)
	require.NotNil(t, server)
	assert.Equal(t, "grpc.health.v1.Health/Check", server.Name())
	assert.Equal(t, client.SpanContext().TraceID(), server.SpanContext().TraceID(), "one trace")
	assert.Equal(t, client.SpanContext().SpanID(), server.Parent().SpanID(), "the server span is a child of the client span")
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
