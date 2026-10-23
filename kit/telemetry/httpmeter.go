package telemetry

import (
	"bufio"
	"context"
	"fmt"
	"net"
	"net/http"
	"strings"

	"github.com/grpc-ecosystem/grpc-gateway/v2/runtime"
	"github.com/slok/go-http-metrics/middleware"
)

// handler returns an measuring standard http.Handler.
func handler(m middleware.Middleware, h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		wi := &responseWriterInterceptor{
			statusCode:     http.StatusOK,
			ResponseWriter: w,
		}
		reporter := &stdReporter{
			w: wi,
			r: r,
		}
		m.Measure(routePattern(r), reporter, func() {
			h.ServeHTTP(wi, r)
		})
	})
}

// Middleware sets up a handler to record metric of the incoming
// requests.
// This middleware will register the route pattern and not the url path
// in order to avoid high cardinality path, so it must be applied on the routed handler
// (after the http.ServeMux or grpc-gateway routing).
//
// Eg:
// mux register GET /users/{id}/devices/{device_id}
// ulr will be /users/12345467/devices/omni_123232
// the metric record will be under /users/{id}/devices/{device_id}
func Middleware(m middleware.Middleware) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return handler(m, next)
	}
}

// GatewayMiddleware sets up a grpc-gateway middleware to record metric of the incoming
// requests under the gateway route pattern (eg: /v1/users/{id}).
func GatewayMiddleware(m middleware.Middleware) runtime.Middleware {
	return func(next runtime.HandlerFunc) runtime.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request, pathParams map[string]string) {
			handler(m, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				next(w, r, pathParams)
			})).ServeHTTP(w, r)
		}
	}
}

// routePattern returns the route pattern matching the request.
// It looks first for the grpc-gateway pattern (the gateway is itself mounted on the http.ServeMux)
// then the http.ServeMux one, and fallback to the url path.
func routePattern(r *http.Request) string {
	if pattern, ok := runtime.HTTPPattern(r.Context()); ok {
		return pattern.String()
	}
	if r.Pattern != "" {
		// http.ServeMux pattern format is [METHOD ][HOST]/[PATH], the method is already
		// recorded in its own label, so we only keep the host and path.
		_, pattern, found := strings.Cut(r.Pattern, " ")
		if !found {
			pattern = r.Pattern
		}
		return pattern
	}
	return r.URL.Path
}

type stdReporter struct {
	w *responseWriterInterceptor
	r *http.Request
}

func (s *stdReporter) Method() string { return s.r.Method }

func (s *stdReporter) Context() context.Context { return s.r.Context() }

func (s *stdReporter) URLPath() string { return s.r.URL.Path }

func (s *stdReporter) StatusCode() int { return s.w.statusCode }

func (s *stdReporter) BytesWritten() int64 { return int64(s.w.bytesWritten) }

// responseWriterInterceptor is a simple wrapper to intercept set data on a
// ResponseWriter.
type responseWriterInterceptor struct {
	http.ResponseWriter
	statusCode   int
	bytesWritten int
}

func (w *responseWriterInterceptor) WriteHeader(statusCode int) {
	w.statusCode = statusCode
	w.ResponseWriter.WriteHeader(statusCode)
}

func (w *responseWriterInterceptor) Write(p []byte) (int, error) {
	w.bytesWritten += len(p)
	return w.ResponseWriter.Write(p)
}

func (w *responseWriterInterceptor) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	h, ok := w.ResponseWriter.(http.Hijacker)
	if !ok {
		return nil, nil, fmt.Errorf("type assertion failed http.ResponseWriter not a http.Hijacker")
	}
	return h.Hijack()
}

func (w *responseWriterInterceptor) Flush() {
	f, ok := w.ResponseWriter.(http.Flusher)
	if !ok {
		return
	}

	f.Flush()
}

// Check interface implementations.
var (
	_ http.ResponseWriter = &responseWriterInterceptor{}
	_ http.Hijacker       = &responseWriterInterceptor{}
	_ http.Flusher        = &responseWriterInterceptor{}
)
