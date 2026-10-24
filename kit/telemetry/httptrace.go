package telemetry

import (
	"context"
	"net/http"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
)

// routeKey is the context key of the route of a request, recorded by Middleware and GatewayMiddleware.
type routeKey struct{}

// TraceHandler traces the requests served by h. The spans are named after the method and the route pattern
// recorded by Middleware or GatewayMiddleware once routed (eg: GET /users/{id}), or after the method only for
// the requests not routed: the url path would make one span name per resource.
func TraceHandler(h http.Handler, operation string) http.Handler {
	traced := otelhttp.NewHandler(h, operation, otelhttp.WithSpanNameFormatter(func(_ string, r *http.Request) string {
		if route, ok := r.Context().Value(routeKey{}).(*string); ok && *route != "" {
			return r.Method + " " + *route
		}
		return r.Method
	}))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		traced.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), routeKey{}, new(string))))
	})
}

// recordRoute records the route of the request for TraceHandler.
func recordRoute(ctx context.Context, route string) {
	if r, ok := ctx.Value(routeKey{}).(*string); ok {
		*r = route
	}
}
