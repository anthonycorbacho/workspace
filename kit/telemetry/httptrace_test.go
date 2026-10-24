package telemetry

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/grpc-ecosystem/grpc-gateway/v2/runtime"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

func TestTraceHandler(t *testing.T) {
	tests := []struct {
		name     string
		url      string
		wantName string
	}{
		{name: "servemux route", url: "/users/42", wantName: "GET /users/{id}"},
		{name: "gateway route mounted on the servemux", url: "/v1/things/42", wantName: "GET /v1/things/{id=*}"},
		{name: "gateway unmatched route", url: "/v1/unknown", wantName: "GET"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			spans := &spanRecorder{}
			prev := otel.GetTracerProvider()
			otel.SetTracerProvider(sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(spans)))
			t.Cleanup(func() { otel.SetTracerProvider(prev) })

			// same setup as the Foundation: the gateway is mounted on the servemux, both traced by TraceHandler.
			_, m := newRecorder()
			gw := runtime.NewServeMux(runtime.WithMiddlewares(GatewayMiddleware(m)))
			require.NoError(t, gw.HandlePath(http.MethodGet, "/v1/things/{id}", func(http.ResponseWriter, *http.Request, map[string]string) {}))
			mux := http.NewServeMux()
			mux.Handle("GET /users/{id}", Middleware(m)(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {})))
			mux.Handle("/", gw)

			req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, tt.url, nil)
			TraceHandler(mux, "test").ServeHTTP(httptest.NewRecorder(), req)

			require.Len(t, spans.spans, 1)
			assert.Equal(t, tt.wantName, spans.spans[0].Name())
		})
	}
}
