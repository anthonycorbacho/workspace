package kit

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"syscall"
	"testing"
	"time"

	pb "github.com/anthonycorbacho/workspace/api/sample/sampleapp/v1"
	"github.com/anthonycorbacho/workspace/kit/errors"
	grpckit "github.com/anthonycorbacho/workspace/kit/grpc"
	"github.com/anthonycorbacho/workspace/kit/log"

	"github.com/grpc-ecosystem/grpc-gateway/v2/runtime"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/trace"
	"google.golang.org/grpc"
	"google.golang.org/grpc/connectivity"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/metadata"
)

// metricHandlers returns the handler label values recorded for the http request duration metric.
func metricHandlers(t *testing.T) map[string]bool {
	t.Helper()
	families, err := prometheus.DefaultGatherer.Gather()
	require.NoError(t, err)
	handlers := map[string]bool{}
	for _, f := range families {
		if f.GetName() != "http_request_duration_seconds" {
			continue
		}
		for _, m := range f.GetMetric() {
			for _, l := range m.GetLabel() {
				if l.GetName() == "handler" {
					handlers[l.GetValue()] = true
				}
			}
		}
	}
	return handlers
}

func TestFoundationHTTP(t *testing.T) {
	// a single foundation is created since the http metrics are registered globally.
	f, err := NewFoundation("test")
	require.NoError(t, err)

	f.RegisterHTTPHandler("/users/{id}", func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, "user "+r.PathValue("id")+strings.Repeat(".", 2048))
	}, http.MethodGet)

	f.RegisterServiceHandler(func(gw *runtime.ServeMux, _ *grpc.ClientConn) {
		err := gw.HandlePath(http.MethodGet, "/v1/things/{id}", func(w http.ResponseWriter, _ *http.Request, p map[string]string) {
			_, _ = io.WriteString(w, "thing "+p["id"])
		})
		require.NoError(t, err)
	})
	h := f.httpServer.Handler

	t.Run("method not allowed", func(t *testing.T) {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/users/42", nil))
		assert.Equal(t, http.StatusMethodNotAllowed, rec.Code)
	})

	// mount the gateway as done in Serve.
	f.httpRouter.Handle("/", f.gw)

	t.Run("route with path value", func(t *testing.T) {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/users/42", nil))
		assert.Equal(t, http.StatusOK, rec.Code)
		assert.True(t, strings.HasPrefix(rec.Body.String(), "user 42"))
	})

	t.Run("unmatched method falls back to grpc gateway", func(t *testing.T) {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/users/42", nil))
		assert.Equal(t, http.StatusNotFound, rec.Code)
	})

	t.Run("gzip compression", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/users/42", nil)
		req.Header.Set("Accept-Encoding", "gzip")
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		require.Equal(t, "gzip", rec.Header().Get("Content-Encoding"))
		zr, err := gzip.NewReader(rec.Body)
		require.NoError(t, err)
		body, err := io.ReadAll(zr)
		require.NoError(t, err)
		assert.True(t, strings.HasPrefix(string(body), "user 42"))
	})

	t.Run("fallback to grpc gateway", func(t *testing.T) {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/v1/things/7", nil))
		assert.Equal(t, http.StatusOK, rec.Code)
		assert.Equal(t, "thing 7", rec.Body.String())
	})

	t.Run("metrics recorded by route pattern", func(t *testing.T) {
		handlers := metricHandlers(t)
		assert.True(t, handlers["/users/{id}"], "got %v", handlers)
		assert.True(t, handlers["/v1/things/{id=*}"], "got %v", handlers)
		assert.False(t, handlers["/users/42"], "got %v", handlers)
		assert.False(t, handlers["/"], "got %v", handlers)
	})
}

func TestInternalHTTP(t *testing.T) {
	r := internalRouter(_defaultHealthHandler, _defaultHealthHandler)

	for _, path := range []string{"/healthz", "/readyz", "/debug/pprof/"} {
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
		assert.Equal(t, http.StatusOK, rec.Code, path)
	}
}

// freeAddr returns a local address with a free port.
func freeAddr(t *testing.T) string {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	addr := l.Addr().String()
	require.NoError(t, l.Close())
	return addr
}

// busyAddr returns a local address with a port already in use for the duration of the test.
func busyAddr(t *testing.T) string {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	t.Cleanup(func() { _ = l.Close() })
	return l.Addr().String()
}

func serveWithTimeout(t *testing.T, f *Foundation) error {
	t.Helper()
	errCh := make(chan error, 1)
	go func() { errCh <- f.Serve() }()
	select {
	case err := <-errCh:
		return err
	case <-time.After(10 * time.Second):
		t.Fatal("Serve did not return")
		return nil
	}
}

func TestServeGRPCAddressInUse(t *testing.T) {
	f, err := NewFoundation("test",
		WithGRPCAddr(busyAddr(t)),
		WithProbesAddr(freeAddr(t)),
		WithMetricsAddr(freeAddr(t)),
	)
	require.NoError(t, err)
	f.RegisterService(func(s *grpc.Server) {
		healthpb.RegisterHealthServer(s, health.NewServer())
	})

	assert.Error(t, serveWithTimeout(t, f))
}

func TestServeMetricsAddressInUse(t *testing.T) {
	f, err := NewFoundation("test",
		WithProbesAddr(freeAddr(t)),
		WithMetricsAddr(busyAddr(t)),
	)
	require.NoError(t, err)

	err = serveWithTimeout(t, f)
	assert.ErrorContains(t, err, "internal server")
}

func TestServeAndShutdown(t *testing.T) {
	probesAddr, metricsAddr, grpcAddr := freeAddr(t), freeAddr(t), freeAddr(t)
	f, err := NewFoundation("test",
		WithGRPCAddr(grpcAddr),
		WithHTTPAddr(freeAddr(t)),
		WithProbesAddr(probesAddr),
		WithMetricsAddr(metricsAddr),
		WithShutdownTimeout(100*time.Millisecond),
	)
	require.NoError(t, err)
	f.RegisterService(func(s *grpc.Server) {
		healthpb.RegisterHealthServer(s, health.NewServer())
	})
	f.RegisterServiceHandler(func(*runtime.ServeMux, *grpc.ClientConn) {})
	require.NotNil(t, f.gwClient)

	errCh := make(chan error, 1)
	go func() { errCh <- f.Serve() }()

	get := func(url string) *http.Response {
		var resp *http.Response
		require.Eventually(t, func() bool {
			r, err := http.Get(url)
			if err != nil {
				return false
			}
			resp = r
			return true
		}, 5*time.Second, 50*time.Millisecond)
		return resp
	}

	resp := get("http://" + probesAddr + "/healthz")
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, "text/plain", resp.Header.Get("Content-Type"))
	_ = resp.Body.Close()

	resp = get("http://" + metricsAddr + "/metrics")
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	_ = resp.Body.Close()
	assert.Contains(t, string(body), "grpc_server_handled_total")

	// An active stream must not keep Serve blocked after SIGTERM.
	conn, err := grpc.NewClient(grpcAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	require.NoError(t, err)
	defer conn.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	stream, err := healthpb.NewHealthClient(conn).Watch(ctx, &healthpb.HealthCheckRequest{})
	require.NoError(t, err)
	_, err = stream.Recv()
	require.NoError(t, err)

	require.NoError(t, syscall.Kill(os.Getpid(), syscall.SIGTERM))
	select {
	case err := <-errCh:
		assert.NoError(t, err)
	case <-time.After(10 * time.Second):
		t.Fatal("Serve did not stop on SIGTERM")
	}
	assert.Equal(t, connectivity.Shutdown, f.gwClient.GetState(), "the gateway client is closed")
	_, err = stream.Recv()
	assert.Error(t, err, "the remaining stream is aborted")
}

func TestReadinessProbe(t *testing.T) {
	h := handlerClosure(func(context.Context) (string, error) { return "", errors.New("not ready") })
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/readyz", nil))
	assert.Equal(t, http.StatusInternalServerError, rec.Code)
	assert.Equal(t, "text/plain", rec.Header().Get("Content-Type"))
	assert.Equal(t, "not ready\n", rec.Body.String())
}

func TestContextAwareProbes(t *testing.T) {
	f, err := NewFoundation("test")
	require.NoError(t, err)

	// Nil probe functions are ignored.
	f.RegisterLiveness(nil)
	f.RegisterReadiness(nil)
	f.RegisterLivenessCheck(nil)
	f.RegisterReadinessCheck(nil)

	type ctxKey struct{}
	f.RegisterLivenessCheck(func(ctx context.Context) (string, error) {
		v, _ := ctx.Value(ctxKey{}).(string)
		return "live:" + v, nil
	})
	f.RegisterReadinessCheck(func(ctx context.Context) (string, error) {
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		default:
			return "ready", nil
		}
	})

	r := internalRouter(f.readinessProbe, f.livenessProbe)

	t.Run("liveness receives request context", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/healthz", nil).WithContext(context.WithValue(context.Background(), ctxKey{}, "ok"))
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)
		assert.Equal(t, http.StatusOK, rec.Code)
		assert.Equal(t, "live:ok\n", rec.Body.String())
	})

	t.Run("readiness observes canceled request context", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		req := httptest.NewRequest(http.MethodGet, "/readyz", nil).WithContext(ctx)
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)
		assert.Equal(t, http.StatusInternalServerError, rec.Code)
		assert.Contains(t, rec.Body.String(), context.Canceled.Error())
	})
}

func TestHTTPPanicRecovery(t *testing.T) {
	var logBuf bytes.Buffer
	l, err := log.New(log.WithOutput(&logBuf), log.WithLevel(log.ErrorLevel))
	require.NoError(t, err)

	f, err := NewFoundation("panic-test", WithLogger(l))
	require.NoError(t, err)

	f.RegisterHTTPHandler("/boom/{id}", func(http.ResponseWriter, *http.Request) {
		panic("secret database credentials")
	}, http.MethodGet)
	f.RegisterHTTPHandler("/partial", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusAccepted)
		_, _ = io.WriteString(w, "partial")
		panic("late failure")
	}, http.MethodGet)
	f.RegisterHTTPHandler("/abort", func(http.ResponseWriter, *http.Request) {
		panic(http.ErrAbortHandler)
	}, http.MethodGet)
	f.RegisterServiceHandler(func(gw *runtime.ServeMux, _ *grpc.ClientConn) {
		require.NoError(t, gw.HandlePath(http.MethodGet, "/v1/boom/{id}", func(http.ResponseWriter, *http.Request, map[string]string) {
			panic("gateway secret")
		}))
	})
	f.httpRouter.Handle("/", f.gw)

	traceID, err := trace.TraceIDFromHex("4bf92f3577b34da6a3ce929d0e0e4736")
	require.NoError(t, err)
	spanID, err := trace.SpanIDFromHex("00f067aa0ba902b7")
	require.NoError(t, err)
	spanCtx := trace.NewSpanContext(trace.SpanContextConfig{
		TraceID:    traceID,
		SpanID:     spanID,
		TraceFlags: trace.FlagsSampled,
	})

	t.Run("custom http handler panic returns 500 without leaking panic value", func(t *testing.T) {
		logBuf.Reset()
		req := httptest.NewRequest(http.MethodGet, "/boom/42", nil)
		req = req.WithContext(trace.ContextWithSpanContext(req.Context(), spanCtx))
		rec := httptest.NewRecorder()
		f.httpServer.Handler.ServeHTTP(rec, req)

		require.Equal(t, http.StatusInternalServerError, rec.Code)
		assert.Equal(t, "application/json", rec.Header().Get("Content-Type"))
		assert.NotContains(t, rec.Body.String(), "secret database credentials")

		var body httpError
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
		assert.Equal(t, errors.InternalCode, body.Code)
		assert.Equal(t, errors.Internal().Message, body.Message)
		assert.Equal(t, traceID.String(), body.TraceID)

		assert.Contains(t, logBuf.String(), "http recover panic")
		assert.Contains(t, logBuf.String(), "secret database credentials")
		assert.True(t, metricHandlers(t)["/boom/{id}"], "metric recorded for panicking route")
	})

	t.Run("panic after writing headers aborts without appending an error", func(t *testing.T) {
		logBuf.Reset()
		rec := httptest.NewRecorder()
		require.PanicsWithValue(t, http.ErrAbortHandler, func() {
			f.httpServer.Handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/partial", nil))
		})

		assert.Equal(t, http.StatusAccepted, rec.Code)
		assert.Equal(t, "partial", rec.Body.String())
		assert.Contains(t, logBuf.String(), "late failure")
	})

	t.Run("http.ErrAbortHandler is re-panicked", func(t *testing.T) {
		rec := httptest.NewRecorder()
		assert.PanicsWithValue(t, http.ErrAbortHandler, func() {
			f.httpServer.Handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/abort", nil))
		})
	})

	t.Run("gateway handler panic returns 500 without leaking panic value", func(t *testing.T) {
		logBuf.Reset()
		rec := httptest.NewRecorder()
		f.httpServer.Handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/v1/boom/9", nil))

		require.Equal(t, http.StatusInternalServerError, rec.Code)
		assert.NotContains(t, rec.Body.String(), "gateway secret")
		var body httpError
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
		assert.Equal(t, errors.InternalCode, body.Code)
		assert.Contains(t, logBuf.String(), "gateway secret")
	})
}

func TestHTTPPanicRecoveryOnWire(t *testing.T) {
	f, err := NewFoundation("panic-stream")
	require.NoError(t, err)
	f.RegisterHTTPHandler("/flush", func(w http.ResponseWriter, _ *http.Request) {
		w.(http.Flusher).Flush() // Commits an implicit 200 without writing a body.
		panic("after flush")
	})
	f.RegisterHTTPHandler("/hints", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusEarlyHints) // A final response is still possible.
		panic("after early hints")
	})
	f.RegisterServiceHandler(func(gw *runtime.ServeMux, _ *grpc.ClientConn) {
		require.NoError(t, gw.HandlePath(http.MethodGet, "/v1/stream", func(w http.ResponseWriter, _ *http.Request, _ map[string]string) {
			_, _ = io.WriteString(w, "first record\n")
			w.(http.Flusher).Flush()
			panic("after first record")
		}))
	})
	t.Cleanup(func() { _ = f.gwClient.Close() })
	f.httpRouter.Handle("/", f.gw)
	srv := httptest.NewServer(f.httpServer.Handler)
	defer srv.Close()
	client := &http.Client{Transport: &http.Transport{DisableCompression: true}, Timeout: 3 * time.Second}
	defer client.CloseIdleConnections()

	for _, tc := range []struct{ path, prefix string }{{"/flush", ""}, {"/v1/stream", "first record\n"}} {
		t.Run(tc.path, func(t *testing.T) {
			res := getHTTPResult(client, srv.URL+tc.path)
			assert.Equal(t, http.StatusOK, res.status)
			assert.Equal(t, tc.prefix, res.body)
			assert.ErrorIs(t, res.err, io.ErrUnexpectedEOF, "the client must see an interrupted response")
		})
	}
	res := getHTTPResult(client, srv.URL+"/hints")
	require.NoError(t, res.err)
	assert.Equal(t, http.StatusInternalServerError, res.status)
	var body httpError
	require.NoError(t, json.Unmarshal([]byte(res.body), &body))
	assert.Equal(t, errors.InternalCode, body.Code)
}

func TestHTTPPanicRecoveryDiscardsStaleHeaders(t *testing.T) {
	for _, tc := range []struct{ header, value string }{
		{"Content-Length", "1"},
		{"Content-Encoding", "gzip"},
	} {
		t.Run(tc.header, func(t *testing.T) {
			f, err := NewFoundation("panic-headers")
			require.NoError(t, err)
			f.RegisterHTTPHandler("/panic-stale-headers", func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set(tc.header, tc.value)
				panic("before the first write")
			})
			srv := httptest.NewServer(f.httpServer.Handler)
			defer srv.Close()
			client := srv.Client() // Let the transport decode gzip as a regular client would.
			client.Timeout = 3 * time.Second

			res := getHTTPResult(client, srv.URL+"/panic-stale-headers")
			require.Equal(t, http.StatusInternalServerError, res.status)
			require.NoError(t, res.err)
			var body httpError
			require.NoError(t, json.Unmarshal([]byte(res.body), &body))
			assert.Equal(t, errors.InternalCode, body.Code)
		})
	}
}

func TestServeContextCancel(t *testing.T) {
	f, err := NewFoundation("test",
		WithProbesAddr(freeAddr(t)),
		WithMetricsAddr(freeAddr(t)),
		WithShutdownTimeout(time.Second),
	)
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	go func() { errCh <- f.ServeContext(ctx) }()

	// Wait for internal server to be listening before canceling.
	require.Eventually(t, func() bool {
		resp, err := http.Get("http://" + f.opts.probesAddr + "/healthz")
		if err != nil {
			return false
		}
		_ = resp.Body.Close()
		return resp.StatusCode == http.StatusOK
	}, 5*time.Second, 20*time.Millisecond)

	cancel()
	select {
	case err := <-errCh:
		assert.NoError(t, err)
	case <-time.After(5 * time.Second):
		t.Fatal("ServeContext did not return after context cancellation")
	}
}

func TestRegisterHTTPHandlerMethods(t *testing.T) {
	f, err := NewFoundation("test")
	require.NoError(t, err)

	ok := func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) }
	f.RegisterHTTPHandler("/any", ok)
	f.RegisterHTTPHandler("/multi", ok, http.MethodGet, http.MethodPost)
	f.RegisterHTTPHandler("/single", ok, http.MethodPut)

	tests := []struct {
		name       string
		method     string
		path       string
		wantStatus int
	}{
		{name: "no methods accepts GET", method: http.MethodGet, path: "/any", wantStatus: http.StatusOK},
		{name: "no methods accepts DELETE", method: http.MethodDelete, path: "/any", wantStatus: http.StatusOK},
		{name: "multiple methods accepts GET", method: http.MethodGet, path: "/multi", wantStatus: http.StatusOK},
		{name: "multiple methods accepts POST", method: http.MethodPost, path: "/multi", wantStatus: http.StatusOK},
		{name: "multiple methods rejects DELETE", method: http.MethodDelete, path: "/multi", wantStatus: http.StatusMethodNotAllowed},
		{name: "single method accepts PUT", method: http.MethodPut, path: "/single", wantStatus: http.StatusOK},
		{name: "single method rejects GET", method: http.MethodGet, path: "/single", wantStatus: http.StatusMethodNotAllowed},
		{name: "unknown path", method: http.MethodGet, path: "/unknown", wantStatus: http.StatusNotFound},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			f.httpServer.Handler.ServeHTTP(rec, httptest.NewRequest(tt.method, tt.path, nil))
			assert.Equal(t, tt.wantStatus, rec.Code)
		})
	}
}

func TestRegisterProbes(t *testing.T) {
	f, err := NewFoundation("test")
	require.NoError(t, err)
	f.RegisterLiveness(func() (string, error) { return "alive", nil })
	f.RegisterReadiness(func() (string, error) { return "", errors.New("warming up") })
	r := internalRouter(f.readinessProbe, f.livenessProbe)

	tests := []struct {
		name       string
		method     string
		path       string
		wantStatus int
		wantBody   string
	}{
		{name: "liveness", method: http.MethodGet, path: "/healthz", wantStatus: http.StatusOK, wantBody: "alive\n"},
		{name: "readiness", method: http.MethodGet, path: "/readyz", wantStatus: http.StatusInternalServerError, wantBody: "warming up\n"},
		{name: "liveness only accepts GET", method: http.MethodPost, path: "/healthz", wantStatus: http.StatusMethodNotAllowed},
		{name: "unknown path", method: http.MethodGet, path: "/unknown", wantStatus: http.StatusNotFound},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			r.ServeHTTP(rec, httptest.NewRequest(tt.method, tt.path, nil))
			assert.Equal(t, tt.wantStatus, rec.Code)
			if tt.wantBody != "" {
				assert.Equal(t, tt.wantBody, rec.Body.String())
				assert.Equal(t, "text/plain", rec.Header().Get("Content-Type"))
			}
		})
	}
}

// metadataServer returns the incoming gRPC metadata as JSON in the response name.
type metadataServer struct {
	pb.UnimplementedSampleAppServiceServer
}

func (metadataServer) Fetch(ctx context.Context, _ *pb.FetchRequest) (*pb.FetchResponse, error) {
	md, _ := metadata.FromIncomingContext(ctx)
	b, err := json.Marshal(md)
	if err != nil {
		return nil, err
	}
	return &pb.FetchResponse{Name: string(b)}, nil
}

func (metadataServer) Delete(context.Context, *pb.DeleteRequest) (*pb.DeleteResponse, error) {
	return &pb.DeleteResponse{}, nil
}

func newSampleGateway(t *testing.T, opts ...Option) http.Handler {
	return newSampleGatewayWithMuxOpts(t, opts, nil)
}

func newSampleGatewayWithMuxOpts(t *testing.T, opts []Option, muxOpts []runtime.ServeMuxOption) http.Handler {
	t.Helper()
	srv := grpckit.NewServer(nil)
	pb.RegisterSampleAppServiceServer(srv, metadataServer{})
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	go func() { _ = srv.Serve(lis) }()
	t.Cleanup(srv.Stop)

	allOpts := append([]Option{WithGRPCAddr(lis.Addr().String())}, opts...)
	f, err := NewFoundation("test", allOpts...)
	require.NoError(t, err)
	f.RegisterServiceHandler(func(gw *runtime.ServeMux, conn *grpc.ClientConn) {
		require.NoError(t, pb.RegisterSampleAppServiceHandler(context.Background(), gw, conn))
	}, muxOpts...)
	require.NoError(t, f.gwErr)
	t.Cleanup(func() { _ = f.gwClient.Close() })
	// mount the gateway as done in Serve.
	f.httpRouter.Handle("/", f.gw)
	return f.httpServer.Handler
}

func TestGatewayMetadata(t *testing.T) {
	h := newSampleGateway(t)
	tests := []struct {
		name       string
		method     string
		target     string
		headers    map[string]string
		wantStatus int
		wantMD     map[string][]string
		notWantMD  []string
		wantBody   string
	}{
		{
			name:   "forwards X- headers, method, pattern and query params",
			method: http.MethodGet,
			target: "/v1/users/abcd?foo=bar&foo=baz",
			headers: map[string]string{
				"X-Request-Id":  "req-1",
				"Custom-Header": "custom",
			},
			wantStatus: http.StatusOK,
			wantMD: map[string][]string{
				"x-request-id": {"req-1"},
				"method":       {http.MethodGet},
				"pattern":      {"/v1/users/{id}"},
				"query-params": {`{"foo":["bar","baz"]}`},
			},
			notWantMD: []string{"custom-header", "grpcgateway-custom-header"},
		},
		{
			name:       "no query params",
			method:     http.MethodGet,
			target:     "/v1/users/abcd",
			wantStatus: http.StatusOK,
			wantMD: map[string][]string{
				"method":       {http.MethodGet},
				"query-params": {`{}`},
			},
		},
		{
			name:       "delete route",
			method:     http.MethodDelete,
			target:     "/v1/users/abcd",
			wantStatus: http.StatusOK,
			wantBody:   `{}`,
		},
		{
			name:       "invalid request rejected by the server validation",
			method:     http.MethodGet,
			target:     "/v1/users/ab",
			wantStatus: http.StatusBadRequest,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, tt.target, nil)
			for k, v := range tt.headers {
				req.Header.Set(k, v)
			}
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)
			require.Equal(t, tt.wantStatus, rec.Code, rec.Body.String())

			if tt.wantBody != "" {
				assert.JSONEq(t, tt.wantBody, rec.Body.String())
			}
			if tt.wantMD == nil {
				return
			}
			var resp struct {
				Name string `json:"name"`
			}
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
			var md map[string][]string
			require.NoError(t, json.Unmarshal([]byte(resp.Name), &md))
			for k, v := range tt.wantMD {
				assert.Equal(t, v, md[k], k)
			}
			for _, k := range tt.notWantMD {
				assert.NotContains(t, md, k)
			}
		})
	}
}

func TestGatewayHeaderHardeningAndOverrides(t *testing.T) {
	fetchMD := func(t *testing.T, h http.Handler, target string, headers map[string]string) map[string][]string {
		t.Helper()
		req := httptest.NewRequest(http.MethodGet, target, nil)
		for k, v := range headers {
			req.Header.Set(k, v)
		}
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

		var resp struct {
			Name string `json:"name"`
		}
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
		var md map[string][]string
		require.NoError(t, json.Unmarshal([]byte(resp.Name), &md))
		return md
	}

	t.Run("allowlist restricts forwarded headers", func(t *testing.T) {
		allowed := []string{"x-request-id"}
		h := newSampleGateway(t, WithGatewayAllowedHeaders(allowed...))
		// mutating the input slice after construction must not affect the allowlist.
		allowed[0] = "x-spoofed-user"

		md := fetchMD(t, h, "/v1/users/abcd", map[string]string{
			"X-Request-Id":   "req-1",
			"X-Spoofed-User": "admin",
		})
		assert.Equal(t, []string{"req-1"}, md["x-request-id"])
		assert.NotContains(t, md, "x-spoofed-user")
	})

	t.Run("empty allowlist blocks all X- headers", func(t *testing.T) {
		h := newSampleGateway(t, WithGatewayAllowedHeaders())
		md := fetchMD(t, h, "/v1/users/abcd", map[string]string{
			"X-Request-Id": "req-1",
		})
		assert.NotContains(t, md, "x-request-id")
	})

	t.Run("caller muxOpts override default header matcher", func(t *testing.T) {
		h := newSampleGatewayWithMuxOpts(t, nil, []runtime.ServeMuxOption{
			runtime.WithIncomingHeaderMatcher(func(key string) (string, bool) {
				if key == "Custom-Allowed" {
					return key, true
				}
				return runtime.DefaultHeaderMatcher(key)
			}),
		})
		md := fetchMD(t, h, "/v1/users/abcd", map[string]string{
			"Custom-Allowed": "yes",
			"X-Request-Id":   "blocked",
		})
		assert.Equal(t, []string{"yes"}, md["custom-allowed"])
		assert.NotContains(t, md, "x-request-id")
	})

	t.Run("oversized query params are omitted from metadata", func(t *testing.T) {
		h := newSampleGateway(t)
		big := strings.Repeat("a", maxQueryParamsMetadataBytes+100)
		md := fetchMD(t, h, "/v1/users/abcd?q="+big, nil)
		assert.Equal(t, []string{http.MethodGet}, md["method"])
		assert.NotContains(t, md, "query-params")
	})
}

func TestServeErrors(t *testing.T) {
	tests := []struct {
		name    string
		options []Option
		setup   func(f *Foundation)
		wantErr string
	}{
		{
			name:    "invalid grpc gateway address",
			options: []Option{WithGRPCAddr("%zz")},
			setup: func(f *Foundation) {
				f.RegisterServiceHandler(func(*runtime.ServeMux, *grpc.ClientConn) {
					t.Error("handler must not be registered without a grpc client")
				})
			},
			wantErr: "creating grpc client for grpc-gateway",
		},
		{
			name:    "invalid grpc gateway credentials",
			options: []Option{WithGatewayDialOptions(grpc.WithTransportCredentials(nil))},
			setup: func(f *Foundation) {
				f.RegisterServiceHandler(func(*runtime.ServeMux, *grpc.ClientConn) {
					t.Error("handler must not be registered without a grpc client")
				})
			},
			wantErr: "no transport security set",
		},
		{
			name: "http address in use",
			setup: func(f *Foundation) {
				f.RegisterHTTPHandler("/ping", func(http.ResponseWriter, *http.Request) {})
			},
			wantErr: "http server",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			options := append([]Option{
				WithHTTPAddr(busyAddr(t)),
				WithProbesAddr(freeAddr(t)),
				WithMetricsAddr(freeAddr(t)),
			}, tt.options...)
			f, err := NewFoundation("test", options...)
			require.NoError(t, err)
			tt.setup(f)

			assert.ErrorContains(t, serveWithTimeout(t, f), tt.wantErr)
		})
	}
}
