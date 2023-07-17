package telemetry

import (
	"bufio"
	"context"
	"fmt"
	"net"
	"net/http"

	"github.com/gorilla/mux"
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
		route := mux.CurrentRoute(r)
		path, err := route.GetPathTemplate()
		if err != nil {
			path, err = route.GetPathRegexp()
			if err != nil {
				path = r.URL.Path
			}
		}

		m.Measure(path, reporter, func() {
			h.ServeHTTP(wi, r)
		})
	})
}

// Middleware sets up a handler to record metric of the incoming
// requests.
// This middleware will register the route template and not the url path.
// in case of high cardinality path
//
// Eg:
// mux register /users/{id}/devices/{device_id}
// ulr will be /users/12345467/devices/omni_123232
// the metric record will be under /users/{id}/devices/{device_id}
func Middleware(m middleware.Middleware) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return handler(m, next)
	}
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
