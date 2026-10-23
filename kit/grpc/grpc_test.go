package grpc

import (
	"context"
	"net"
	"testing"

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
