package kit

import (
	"context"
	"io"
	"net"
	"net/http"
	"sync"
	"testing"
	"time"

	pb "github.com/anthonycorbacho/workspace/api/sample/sampleapp/v1"
	"github.com/grpc-ecosystem/grpc-gateway/v2/runtime"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/connectivity"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
)

// awaitResult bounds waits on server and request goroutines, including when shutdown is broken.
func awaitResult[T any](t *testing.T, result <-chan T) T {
	t.Helper()
	select {
	case value := <-result:
		return value
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for operation")
		var zero T
		return zero
	}
}

func serveTestGRPC(t *testing.T, f *Foundation) *grpc.ClientConn {
	t.Helper()
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	f.opts.grpcAddr = lis.Addr().String()
	done := make(chan error, 1)
	go func() { done <- f.grpcServer.Serve(lis) }()
	t.Cleanup(func() {
		f.grpcServer.Stop()
		assert.NoError(t, awaitResult(t, done))
	})

	conn, err := grpc.NewClient(f.opts.grpcAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })
	return conn
}

// serveTestHTTP exposes the listener's closure separately from the draining of active handlers.
func serveTestHTTP(t *testing.T, srv *http.Server) (string, <-chan struct{}) {
	t.Helper()
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	stopped := make(chan struct{})
	go func() {
		assert.ErrorIs(t, srv.Serve(lis), http.ErrServerClosed)
		close(stopped)
	}()
	t.Cleanup(func() {
		_ = srv.Close()
		awaitResult(t, stopped)
	})
	return "http://" + lis.Addr().String(), stopped
}

type httpResult struct {
	status int
	body   string
	err    error
}

func getHTTPResult(client *http.Client, url string) httpResult {
	resp, err := client.Get(url)
	if err != nil {
		return httpResult{err: err}
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	return httpResult{status: resp.StatusCode, body: string(body), err: err}
}

func TestShutdownDrainsGatewayBeforeGRPC(t *testing.T) {
	f, err := NewFoundation("shutdown", WithShutdownTimeout(5*time.Second))
	require.NoError(t, err)
	f.RegisterService(func(s *grpc.Server) {
		pb.RegisterSampleAppServiceServer(s, metadataServer{})
	})
	serveTestGRPC(t, f)
	f.RegisterServiceHandler(func(gw *runtime.ServeMux, conn *grpc.ClientConn) {
		require.NoError(t, pb.RegisterSampleAppServiceHandler(context.Background(), gw, conn))
	})
	t.Cleanup(func() { _ = f.gwClient.Close() })
	f.httpRouter.Handle("/", f.gw)

	// Hold an accepted HTTP request before it reaches the generated gateway handler.
	// It must still be able to start its RPC after shutdown starts draining HTTP.
	entered := make(chan struct{})
	proceed := make(chan struct{})
	release := sync.OnceFunc(func() { close(proceed) })
	defer release()
	h := f.httpServer.Handler
	f.httpServer.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(entered)
		select {
		case <-proceed:
			h.ServeHTTP(w, r)
		case <-r.Context().Done():
		}
	})
	url, httpStopped := serveTestHTTP(t, f.httpServer)
	f.RegisterReadiness(func() (string, error) { return "custom ready", nil })
	probes := &http.Server{Handler: internalRouter(f.handleReadiness, f.livenessProbe)}
	probesURL, probesStopped := serveTestHTTP(t, probes)
	metrics := &http.Server{Handler: metricsRouter()}
	metricsURL, metricsStopped := serveTestHTTP(t, metrics)

	client := &http.Client{Timeout: 5 * time.Second, Transport: &http.Transport{DisableKeepAlives: true}}
	t.Cleanup(client.CloseIdleConnections)
	ready := getHTTPResult(client, probesURL+"/readyz")
	require.NoError(t, ready.err)
	assert.Equal(t, http.StatusOK, ready.status)
	assert.Equal(t, "custom ready\n", ready.body)

	response := make(chan httpResult, 1)
	go func() { response <- getHTTPResult(client, url+"/v1/users/abcd") }()
	awaitResult(t, entered)
	shutdownDone := make(chan struct{})
	go func() {
		f.shutdown(probes, metrics)
		close(shutdownDone)
	}()
	awaitResult(t, httpStopped)

	ready = getHTTPResult(client, probesURL+"/readyz")
	require.NoError(t, ready.err)
	assert.Equal(t, http.StatusServiceUnavailable, ready.status)
	assert.Equal(t, "shutting down\n", ready.body)
	for _, endpoint := range []string{probesURL + "/healthz", metricsURL + "/metrics"} {
		result := getHTTPResult(client, endpoint)
		require.NoError(t, result.err)
		assert.Equal(t, http.StatusOK, result.status)
	}
	select {
	case <-shutdownDone:
		t.Fatal("shutdown returned before the accepted HTTP request finished")
	default:
	}

	release()
	result := awaitResult(t, response)
	require.NoError(t, result.err)
	assert.Equal(t, http.StatusOK, result.status, result.body)
	awaitResult(t, shutdownDone)
	assert.Equal(t, connectivity.Shutdown, f.gwClient.GetState())
	awaitResult(t, probesStopped)
	awaitResult(t, metricsStopped)
}

// blockedHealthServer intentionally ignores cancellation so shutdown cannot wait for its handler.
type blockedHealthServer struct {
	healthpb.UnimplementedHealthServer
	entered chan context.Context
	release <-chan struct{}
}

func (s *blockedHealthServer) Check(ctx context.Context, _ *healthpb.HealthCheckRequest) (*healthpb.HealthCheckResponse, error) {
	s.entered <- ctx
	<-s.release
	return &healthpb.HealthCheckResponse{Status: healthpb.HealthCheckResponse_SERVING}, nil
}

func TestShutdownBoundsBlockedRPC(t *testing.T) {
	for _, waitForHandlers := range []bool{false, true} {
		name := "default server"
		if waitForHandlers {
			name = "server waits for handlers"
		}
		t.Run(name, func(t *testing.T) {
			const gracePeriod = 100 * time.Millisecond
			f, err := NewFoundation("shutdown", WithShutdownTimeout(gracePeriod),
				WithGRPCServerOptions(grpc.WaitForHandlers(waitForHandlers)))
			require.NoError(t, err)
			release := make(chan struct{})
			defer close(release)
			handler := &blockedHealthServer{entered: make(chan context.Context, 1), release: release}
			f.RegisterService(func(s *grpc.Server) { healthpb.RegisterHealthServer(s, handler) })
			conn := serveTestGRPC(t, f)
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			rpcDone := make(chan error, 1)
			go func() {
				_, err := healthpb.NewHealthClient(conn).Check(ctx, &healthpb.HealthCheckRequest{})
				rpcDone <- err
			}()
			handlerCtx := awaitResult(t, handler.entered)

			shutdownDone := make(chan struct{})
			started := time.Now()
			go func() {
				f.shutdown()
				close(shutdownDone)
			}()
			awaitResult(t, shutdownDone)
			assert.GreaterOrEqual(t, time.Since(started), gracePeriod, "the RPC gets its grace period")
			assert.Error(t, awaitResult(t, rpcDone), "the remaining RPC is aborted")
			awaitResult(t, handlerCtx.Done())
		})
	}
}

func TestShutdownForcesHTTPAndGRPC(t *testing.T) {
	f, err := NewFoundation("shutdown", WithShutdownTimeout(100*time.Millisecond))
	require.NoError(t, err)
	f.RegisterService(func(s *grpc.Server) { healthpb.RegisterHealthServer(s, health.NewServer()) })
	conn := serveTestGRPC(t, f)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	stream, err := healthpb.NewHealthClient(conn).Watch(ctx, &healthpb.HealthCheckRequest{})
	require.NoError(t, err)
	_, err = stream.Recv()
	require.NoError(t, err)

	entered := make(chan struct{})
	canceled := make(chan struct{})
	f.RegisterHTTPHandler("/blocked", func(_ http.ResponseWriter, r *http.Request) {
		close(entered)
		<-r.Context().Done()
		close(canceled)
	})
	url, httpStopped := serveTestHTTP(t, f.httpServer)
	client := &http.Client{Timeout: 5 * time.Second, Transport: &http.Transport{DisableKeepAlives: true}}
	t.Cleanup(client.CloseIdleConnections)
	response := make(chan httpResult, 1)
	go func() { response <- getHTTPResult(client, url+"/blocked") }()
	awaitResult(t, entered)

	shutdownDone := make(chan struct{})
	go func() {
		f.shutdown()
		close(shutdownDone)
	}()
	awaitResult(t, shutdownDone)
	awaitResult(t, httpStopped)
	awaitResult(t, canceled)
	assert.Error(t, awaitResult(t, response).err, "the HTTP connection is forcibly closed")
	streamDone := make(chan error, 1)
	go func() {
		_, err := stream.Recv()
		streamDone <- err
	}()
	assert.Error(t, awaitResult(t, streamDone), "the RPC is aborted after HTTP exhausts the grace period")
}
