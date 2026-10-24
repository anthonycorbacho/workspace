package kit

import (
	"context"
	"encoding/json"
	"net/http"

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

// gatewayErrorHandler writes the errors of the gRPC gateway as a coded error:
// the HTTP status is derived from the gRPC status code and the body is an httpError.
//
// Errors produced by the gateway itself (eg: unknown route, malformed JSON body) don't have
// a coded error, they are written as an Unclassified error keeping their status and message.
func gatewayErrorHandler(l *log.Logger) runtime.ErrorHandlerFunc {
	return func(ctx context.Context, _ *runtime.ServeMux, _ runtime.Marshaler, w http.ResponseWriter, _ *http.Request, err error) {
		coded := grpckit.FromError(err)

		body := httpError{
			Code:    coded.Code,
			Message: coded.Message,
			Details: coded.Details,
		}
		if sc := trace.SpanContextFromContext(ctx); sc.HasTraceID() {
			body.TraceID = sc.TraceID().String()
		}

		// headers must be set before writing the status.
		w.Header().Del("Trailer")
		w.Header().Del("Transfer-Encoding")
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(runtime.HTTPStatusFromCode(grpckit.Code(coded.Kind)))

		if err := json.NewEncoder(w).Encode(body); err != nil {
			l.Warn(ctx, "writing gateway error response", log.Error(err))
		}
	}
}
