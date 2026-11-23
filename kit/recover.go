package kit

import (
	"bufio"
	"fmt"
	"net"
	"net/http"

	"github.com/anthonycorbacho/workspace/kit/errors"
	"github.com/anthonycorbacho/workspace/kit/log"
	"github.com/grpc-ecosystem/grpc-gateway/v2/runtime"
	otelcodes "go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

// recoverHTTP recovers panics in next, logs them with trace correlation, marks the active span as errored,
// and returns a coded 500 if the response is still uncommitted. Otherwise it aborts the stream.
// An explicit http.ErrAbortHandler passes through without being logged again.
func recoverHTTP(l *log.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rw := &panicResponseWriter{ResponseWriter: w}
		defer func() {
			p := recover()
			if p == nil {
				return
			}
			if p == http.ErrAbortHandler {
				panic(p)
			}
			ctx := r.Context()
			l.Error(ctx, "http recover panic", log.Any("panic", p))
			trace.SpanFromContext(ctx).SetStatus(otelcodes.Error, "http recover panic")
			if rw.committed {
				panic(http.ErrAbortHandler)
			}
			internal := errors.Internal()
			writeGatewayError(ctx, l, rw, http.StatusInternalServerError, httpError{
				Code:    internal.Code,
				Message: internal.Message,
			})
		}()
		next.ServeHTTP(rw, r)
	})
}

// recoverGatewayMiddleware recovers panics inside grpc-gateway handlers so the gateway metrics
// middleware records the resulting HTTP 500 status under the matched route pattern.
func recoverGatewayMiddleware(l *log.Logger) runtime.Middleware {
	return func(next runtime.HandlerFunc) runtime.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request, pathParams map[string]string) {
			recoverHTTP(l, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				next(w, r, pathParams)
			})).ServeHTTP(w, r)
		}
	}
}

type panicResponseWriter struct {
	http.ResponseWriter
	committed bool
}

func (w *panicResponseWriter) WriteHeader(code int) {
	w.ResponseWriter.WriteHeader(code)
	// Informational responses other than 101 can still be followed by a final status.
	if code >= 200 || code == http.StatusSwitchingProtocols {
		w.committed = true
	}
}

func (w *panicResponseWriter) Write(p []byte) (int, error) {
	w.committed = true
	return w.ResponseWriter.Write(p)
}

func (w *panicResponseWriter) Unwrap() http.ResponseWriter {
	return w.ResponseWriter
}

func (w *panicResponseWriter) Flush() {
	if f, ok := w.ResponseWriter.(http.Flusher); ok {
		w.committed = true
		f.Flush()
	}
}

func (w *panicResponseWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	h, ok := w.ResponseWriter.(http.Hijacker)
	if !ok {
		return nil, nil, fmt.Errorf("response writer does not implement http.Hijacker")
	}
	conn, rw, err := h.Hijack()
	if err == nil {
		w.committed = true
	}
	return conn, rw, err
}

var (
	_ http.ResponseWriter = (*panicResponseWriter)(nil)
	_ http.Flusher        = (*panicResponseWriter)(nil)
	_ http.Hijacker       = (*panicResponseWriter)(nil)
)
