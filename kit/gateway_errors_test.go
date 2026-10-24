package kit

import (
	"context"
	"encoding/json"
	stderrors "errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/anthonycorbacho/workspace/kit/errors"
	grpckit "github.com/anthonycorbacho/workspace/kit/grpc"
	"github.com/anthonycorbacho/workspace/kit/log"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/trace"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestGatewayErrorHandler(t *testing.T) {
	tests := []struct {
		name       string
		err        error
		wantStatus int
		wantBody   httpError
	}{
		{
			name:       "coded error",
			err:        grpckit.Status(errors.SampleUserNotFound("42")).Err(),
			wantStatus: http.StatusNotFound,
			wantBody:   httpError{Code: errors.SampleUserNotFoundCode, Message: "user 42 not found", Details: map[string]any{"id": "42"}},
		},
		{
			name:       "invalid request with violations",
			err:        grpckit.Status(errors.InvalidRequest().WithDetail("violations", []any{map[string]any{"field": "id"}})).Err(),
			wantStatus: http.StatusBadRequest,
			wantBody: httpError{Code: errors.InvalidRequestCode, Message: "invalid request", Details: map[string]any{
				"violations": []any{map[string]any{"field": "id"}},
			}},
		},
		{
			name:       "gateway error without coded error",
			err:        status.Error(codes.NotFound, "Not Found"),
			wantStatus: http.StatusNotFound,
			wantBody:   httpError{Code: errors.UnclassifiedCode, Message: "Not Found", Details: map[string]any{}},
		},
		{
			name:       "backend unavailable",
			err:        status.Error(codes.Unavailable, "connection refused"),
			wantStatus: http.StatusServiceUnavailable,
			wantBody:   httpError{Code: errors.UnclassifiedCode, Message: "connection refused", Details: map[string]any{}},
		},
		{
			name:       "plain error is internal and hidden",
			err:        stderrors.New("password=secret"),
			wantStatus: http.StatusInternalServerError,
			wantBody:   httpError{Code: errors.InternalCode, Message: "internal error", Details: map[string]any{}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			rec.Header().Set("Trailer", "Grpc-Status")
			gatewayErrorHandler(log.NewNop())(context.Background(), nil, nil, rec, httptest.NewRequest(http.MethodGet, "/", nil), tt.err)

			assert.Equal(t, tt.wantStatus, rec.Code)
			assert.Equal(t, "application/json", rec.Header().Get("Content-Type"))
			assert.Empty(t, rec.Header().Get("Trailer"))

			var body httpError
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
			assert.Equal(t, tt.wantBody, body)
			assert.NotContains(t, rec.Body.String(), "secret")
		})
	}
}

func TestGatewayErrorHandlerTraceID(t *testing.T) {
	traceID := trace.TraceID{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16}
	sc := trace.NewSpanContext(trace.SpanContextConfig{TraceID: traceID, SpanID: trace.SpanID{1}})
	ctx := trace.ContextWithSpanContext(context.Background(), sc)

	rec := httptest.NewRecorder()
	gatewayErrorHandler(log.NewNop())(ctx, nil, nil, rec, httptest.NewRequest(http.MethodGet, "/", nil), errors.Internal())

	var body httpError
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	assert.Equal(t, traceID.String(), body.TraceID)
}
