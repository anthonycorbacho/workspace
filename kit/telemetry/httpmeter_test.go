package telemetry

import (
	"bufio"
	"context"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/grpc-ecosystem/grpc-gateway/v2/runtime"
	"github.com/slok/go-http-metrics/metrics"
	"github.com/slok/go-http-metrics/middleware"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.43.0"
)

// recorder records the request properties measured by the middleware.
type recorder struct {
	mu    sync.Mutex
	props []metrics.HTTPReqProperties
	sizes []int64
}

func (r *recorder) ObserveHTTPRequestDuration(_ context.Context, props metrics.HTTPReqProperties, _ time.Duration) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.props = append(r.props, props)
}

func (r *recorder) ObserveHTTPResponseSize(_ context.Context, _ metrics.HTTPReqProperties, size int64) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.sizes = append(r.sizes, size)
}

func (r *recorder) AddInflightRequests(context.Context, metrics.HTTPProperties, int) {}

func newRecorder() (*recorder, middleware.Middleware) {
	rec := &recorder{}
	return rec, middleware.New(middleware.Config{Recorder: rec})
}

func TestRoutePattern(t *testing.T) {
	tests := []struct {
		name    string
		pattern string
		url     string
		want    string
	}{
		{name: "pattern with method", pattern: "GET /users/{id}", url: "/users/42", want: "/users/{id}"},
		{name: "pattern without method", pattern: "/users/{id}", url: "/users/42", want: "/users/{id}"},
		{name: "pattern with method and host", pattern: "GET example.com/users/{id}", url: "/users/42", want: "example.com/users/{id}"},
		{name: "pattern with host", pattern: "example.com/users/", url: "/users/42", want: "example.com/users/"},
		{name: "no pattern falls back to url path", url: "/users/42", want: "/users/42"},
		{name: "no pattern with query", url: "/users/42?expand=true", want: "/users/42"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodGet, tt.url, nil)
			r.Pattern = tt.pattern
			assert.Equal(t, tt.want, routePattern(r))
		})
	}
}

func TestMiddleware(t *testing.T) {
	tests := []struct {
		name     string
		pattern  string
		method   string
		url      string
		handler  http.HandlerFunc
		wantID   string
		wantCode int
		wantSize int64
	}{
		{
			name:    "servemux pattern with method",
			pattern: "GET /users/{id}",
			method:  http.MethodGet,
			url:     "/users/42",
			handler: func(w http.ResponseWriter, r *http.Request) {
				_, _ = io.WriteString(w, "user "+r.PathValue("id"))
			},
			wantID:   "/users/{id}",
			wantCode: http.StatusOK,
			wantSize: int64(len("user 42")),
		},
		{
			name:    "servemux pattern without method",
			pattern: "/things/{id}",
			method:  http.MethodPost,
			url:     "/things/1",
			handler: func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusCreated)
			},
			wantID:   "/things/{id}",
			wantCode: http.StatusCreated,
		},
		{
			name:    "servemux wildcard pattern",
			pattern: "GET /files/{path...}",
			method:  http.MethodGet,
			url:     "/files/a/b/c",
			handler: func(w http.ResponseWriter, _ *http.Request) {
				http.Error(w, "not found", http.StatusNotFound)
			},
			wantID:   "/files/{path...}",
			wantCode: http.StatusNotFound,
			wantSize: int64(len("not found\n")),
		},
		{
			name:   "not routed falls back to url path",
			method: http.MethodDelete,
			url:    "/users/42",
			handler: func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusNoContent)
			},
			wantID:   "/users/42",
			wantCode: http.StatusNoContent,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec, m := newRecorder()
			var h http.Handler = Middleware(m)(tt.handler)
			if tt.pattern != "" {
				mux := http.NewServeMux()
				mux.Handle(tt.pattern, h)
				h = mux
			}

			w := httptest.NewRecorder()
			h.ServeHTTP(w, httptest.NewRequest(tt.method, tt.url, nil))
			assert.Equal(t, tt.wantCode, w.Code, "status code is forwarded")

			require.Len(t, rec.props, 1)
			assert.Equal(t, tt.wantID, rec.props[0].ID)
			assert.Equal(t, tt.method, rec.props[0].Method)
			assert.Equal(t, strconv.Itoa(tt.wantCode), rec.props[0].Code)
			require.Len(t, rec.sizes, 1)
			assert.Equal(t, tt.wantSize, rec.sizes[0])
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

func TestMiddlewareSpanName(t *testing.T) {
	tests := []struct {
		name      string
		pattern   string
		url       string
		wantName  string
		wantRoute string
	}{
		{name: "named after the route pattern", pattern: "GET /users/{id}", url: "/users/42", wantName: "GET /users/{id}", wantRoute: "/users/{id}"},
		{name: "not routed falls back to url path", url: "/users/42", wantName: "GET /users/42", wantRoute: "/users/42"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			spans := &spanRecorder{}
			tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(spans))
			_, m := newRecorder()
			var h http.Handler = Middleware(m)(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
			if tt.pattern != "" {
				mux := http.NewServeMux()
				mux.Handle(tt.pattern, h)
				h = mux
			}

			ctx, span := tp.Tracer("test").Start(context.Background(), "GET")
			h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequestWithContext(ctx, http.MethodGet, tt.url, nil))
			span.End()

			require.Len(t, spans.spans, 1)
			assert.Equal(t, tt.wantName, spans.spans[0].Name())
			assert.Contains(t, spans.spans[0].Attributes(), semconv.HTTPRoute(tt.wantRoute))
		})
	}
}

func TestGatewayMiddleware(t *testing.T) {
	tests := []struct {
		name      string
		path      string
		url       string
		mounted   bool
		wantID    string
		wantCode  int
		wantBody  string
		wantCalls int
	}{
		{
			name:      "gateway pattern",
			path:      "/v1/things/{id}",
			url:       "/v1/things/42",
			wantID:    "/v1/things/{id=*}",
			wantCode:  http.StatusOK,
			wantBody:  "thing 42",
			wantCalls: 1,
		},
		{
			name:      "gateway pattern takes precedence over the servemux one",
			path:      "/v1/things/{id}",
			url:       "/v1/things/42",
			mounted:   true,
			wantID:    "/v1/things/{id=*}",
			wantCode:  http.StatusOK,
			wantBody:  "thing 42",
			wantCalls: 1,
		},
		{
			name:      "literal gateway pattern",
			path:      "/v1/health",
			url:       "/v1/health",
			wantID:    "/v1/health",
			wantCode:  http.StatusOK,
			wantBody:  "thing ",
			wantCalls: 1,
		},
		{
			name:     "unmatched route is not measured",
			path:     "/v1/things/{id}",
			url:      "/v1/unknown",
			wantCode: http.StatusNotFound,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec, m := newRecorder()
			gw := runtime.NewServeMux(runtime.WithMiddlewares(GatewayMiddleware(m)))
			err := gw.HandlePath(http.MethodGet, tt.path, func(w http.ResponseWriter, _ *http.Request, p map[string]string) {
				_, _ = io.WriteString(w, "thing "+p["id"])
			})
			require.NoError(t, err)

			var h http.Handler = gw
			if tt.mounted {
				mux := http.NewServeMux()
				mux.Handle("/", gw)
				h = mux
			}

			w := httptest.NewRecorder()
			h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, tt.url, nil))
			assert.Equal(t, tt.wantCode, w.Code)

			require.Len(t, rec.props, tt.wantCalls)
			if tt.wantCalls == 0 {
				return
			}
			assert.Equal(t, tt.wantBody, w.Body.String())
			assert.Equal(t, tt.wantID, rec.props[0].ID)
			assert.Equal(t, http.MethodGet, rec.props[0].Method)
			assert.Equal(t, strconv.Itoa(tt.wantCode), rec.props[0].Code)
			assert.Equal(t, []int64{int64(len(tt.wantBody))}, rec.sizes)
		})
	}
}

// plainWriter is a http.ResponseWriter that is neither a http.Flusher nor a http.Hijacker.
type plainWriter struct {
	header http.Header
	status int
	body   []byte
}

func (w *plainWriter) Header() http.Header { return w.header }

func (w *plainWriter) Write(p []byte) (int, error) {
	w.body = append(w.body, p...)
	return len(p), nil
}

func (w *plainWriter) WriteHeader(status int) { w.status = status }

func TestResponseWriterInterceptor(t *testing.T) {
	tests := []struct {
		name      string
		write     func(w http.ResponseWriter)
		wantCode  int
		wantBytes int
	}{
		{name: "nothing written", write: func(http.ResponseWriter) {}, wantCode: http.StatusOK},
		{
			name:      "body only",
			write:     func(w http.ResponseWriter) { _, _ = w.Write([]byte("hello")) },
			wantCode:  http.StatusOK,
			wantBytes: 5,
		},
		{
			name: "status and body in several writes",
			write: func(w http.ResponseWriter) {
				w.WriteHeader(http.StatusAccepted)
				_, _ = w.Write([]byte("hello"))
				_, _ = w.Write([]byte(" world"))
			},
			wantCode:  http.StatusAccepted,
			wantBytes: 11,
		},
		{
			name:     "error status",
			write:    func(w http.ResponseWriter) { w.WriteHeader(http.StatusInternalServerError) },
			wantCode: http.StatusInternalServerError,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			wi := &responseWriterInterceptor{statusCode: http.StatusOK, ResponseWriter: rec}
			tt.write(wi)

			assert.Equal(t, tt.wantCode, wi.statusCode)
			assert.Equal(t, tt.wantBytes, wi.bytesWritten)
			assert.Equal(t, tt.wantCode, rec.Code)
			assert.Equal(t, tt.wantBytes, rec.Body.Len())

			reporter := &stdReporter{w: wi, r: httptest.NewRequest(http.MethodPut, "/path", nil)}
			assert.Equal(t, tt.wantCode, reporter.StatusCode())
			assert.Equal(t, int64(tt.wantBytes), reporter.BytesWritten())
			assert.Equal(t, http.MethodPut, reporter.Method())
			assert.Equal(t, "/path", reporter.URLPath())
			assert.NotNil(t, reporter.Context())
		})
	}
}

func TestResponseWriterInterceptorFlush(t *testing.T) {
	t.Run("flusher", func(t *testing.T) {
		rec := httptest.NewRecorder()
		wi := &responseWriterInterceptor{statusCode: http.StatusOK, ResponseWriter: rec}
		wi.Flush()
		assert.True(t, rec.Flushed)
	})

	t.Run("not a flusher", func(t *testing.T) {
		wi := &responseWriterInterceptor{statusCode: http.StatusOK, ResponseWriter: &plainWriter{header: http.Header{}}}
		assert.NotPanics(t, wi.Flush)
	})
}

func TestResponseWriterInterceptorHijack(t *testing.T) {
	t.Run("not a hijacker", func(t *testing.T) {
		wi := &responseWriterInterceptor{statusCode: http.StatusOK, ResponseWriter: httptest.NewRecorder()}
		conn, rw, err := wi.Hijack()
		assert.Error(t, err)
		assert.Nil(t, conn)
		assert.Nil(t, rw)
	})

	t.Run("hijacker", func(t *testing.T) {
		_, m := newRecorder()
		srv := httptest.NewServer(Middleware(m)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			h, ok := w.(http.Hijacker)
			if !assert.True(t, ok) {
				return
			}
			conn, rw, err := h.Hijack()
			if !assert.NoError(t, err) {
				return
			}
			defer func() { _ = conn.Close() }()
			_, _ = rw.WriteString("HTTP/1.1 200 OK\r\nContent-Length: 8\r\nConnection: close\r\n\r\nhijacked")
			_ = rw.Flush()
		})))
		t.Cleanup(srv.Close)

		conn, err := net.Dial("tcp", srv.Listener.Addr().String())
		require.NoError(t, err)
		defer func() { _ = conn.Close() }()
		_, err = io.WriteString(conn, "GET / HTTP/1.1\r\nHost: test\r\n\r\n")
		require.NoError(t, err)

		resp, err := http.ReadResponse(bufio.NewReader(conn), nil)
		require.NoError(t, err)
		defer func() { _ = resp.Body.Close() }()
		body, err := io.ReadAll(resp.Body)
		require.NoError(t, err)
		assert.Equal(t, "hijacked", string(body))
	})
}
