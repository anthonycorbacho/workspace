package kit

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/anthonycorbacho/workspace/kit/errors"
	grpckit "github.com/anthonycorbacho/workspace/kit/grpc"
	"github.com/anthonycorbacho/workspace/kit/log"
	"github.com/grpc-ecosystem/grpc-gateway/v2/runtime"
	"go.opentelemetry.io/otel/trace"
)

// httpError is the body of an HTTP error response of the gRPC gateway.
type httpError struct {
	// Code is the unique error code (see kit/errors).
	Code string `json:"code"`
	// Message is the client facing message.
	Message string `json:"message"`
	// Details are the client facing structured details.
	Details map[string]any `json:"details"`
	// TraceID identifies the request, it can be given to the support to find the request logs and traces.
	TraceID string `json:"trace_id,omitempty"`
}

// gatewayErrorHandler maps RPC errors to their HTTP status and public coded error.
func gatewayErrorHandler(l *log.Logger) runtime.ErrorHandlerFunc {
	return func(ctx context.Context, _ *runtime.ServeMux, _ runtime.Marshaler, w http.ResponseWriter, _ *http.Request, err error) {
		coded := grpckit.FromError(err)
		if coded.Kind == errors.KindUnavailable {
			l.Warn(ctx, "gateway backend unavailable", log.Error(err))
		}

		writeGatewayError(ctx, l, w, runtime.HTTPStatusFromCode(grpckit.Code(coded.Kind)), httpError{
			Code:    coded.Code,
			Message: coded.Message,
			Details: coded.Details,
		})
	}
}

// Routing errors already have an HTTP status. Passing 405 through gRPC would turn it into 501.
func gatewayRoutingErrorHandler(l *log.Logger) runtime.RoutingErrorHandlerFunc {
	return func(ctx context.Context, _ *runtime.ServeMux, _ runtime.Marshaler, w http.ResponseWriter, _ *http.Request, status int) {
		writeGatewayError(ctx, l, w, status, httpError{Code: errors.UnclassifiedCode, Message: http.StatusText(status)})
	}
}

func writeGatewayError(ctx context.Context, l *log.Logger, w http.ResponseWriter, status int, body httpError) {
	if body.Details == nil {
		body.Details = map[string]any{}
	}
	if sc := trace.SpanContextFromContext(ctx); sc.HasTraceID() {
		body.TraceID = sc.TraceID().String()
	}
	// The handler may have set headers for a different body before it failed.
	header := w.Header()
	header.Del("Content-Length")
	header.Del("Content-Encoding")
	header.Del("Trailer")
	header.Del("Transfer-Encoding")
	header.Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(body); err != nil {
		l.Warn(ctx, "writing gateway error response", log.Error(err))
	}
}
