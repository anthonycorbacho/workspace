package kit

import (
	"compress/gzip"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/grpc-ecosystem/grpc-gateway/v2/runtime"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
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
