package kit

import (
	"bytes"
	"context"
	"encoding/json"
	stderrors "errors"
	"net/http"
	"net/http/httptest"
	"strings"
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
			wantBody:   httpError{Code: errors.UnclassifiedCode, Message: "service unavailable", Details: map[string]any{}},
		},
		{
			name:       "unavailable with an explicit public message",
			err:        grpckit.Status(errors.NewUnclassified(errors.KindUnavailable, "scheduled maintenance")).Err(),
			wantStatus: http.StatusServiceUnavailable,
			wantBody:   httpError{Code: errors.UnclassifiedCode, Message: "scheduled maintenance", Details: map[string]any{}},
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

func TestGatewayUnavailableTraceID(t *testing.T) {
	traceID := trace.TraceID{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16}
	sc := trace.NewSpanContext(trace.SpanContextConfig{TraceID: traceID, SpanID: trace.SpanID{1}})
	ctx := trace.ContextWithSpanContext(context.Background(), sc)

	var logs bytes.Buffer
	l, err := log.New(log.WithOutput(&logs), log.WithLevel(log.WarnLevel))
	require.NoError(t, err)
	t.Cleanup(l.Close)
	const diagnostic = "dial tcp backend.internal:8081: connection refused"
	rec := httptest.NewRecorder()
	gatewayErrorHandler(l)(ctx, nil, nil, rec, httptest.NewRequest(http.MethodGet, "/", nil), status.Error(codes.Unavailable, diagnostic))

	var body httpError
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	assert.Equal(t, http.StatusServiceUnavailable, rec.Code)
	assert.Equal(t, traceID.String(), body.TraceID)
	assert.NotContains(t, rec.Body.String(), diagnostic)
	assert.Contains(t, logs.String(), diagnostic)
	assert.Contains(t, logs.String(), traceID.String())
}

func TestGatewayRoutingErrors(t *testing.T) {
	h := newSampleGateway(t)
	for _, tt := range []struct {
		method, path, body string
		status             int
		message            string
	}{
		{http.MethodPut, "/v1/users/abcd", "", http.StatusMethodNotAllowed, "Method Not Allowed"},
		{http.MethodGet, "/v1/unknown", "", http.StatusNotFound, "Not Found"},
		// The route is registered, but the sample test server does not implement Create.
		{http.MethodPost, "/v1/users", `{"name":"abcd"}`, http.StatusNotImplemented, "method Create not implemented"},
	} {
		t.Run(tt.method+" "+tt.path, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, tt.path, strings.NewReader(tt.body))
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)
			require.Equal(t, tt.status, rec.Code, rec.Body.String())
			assert.Equal(t, "application/json", rec.Header().Get("Content-Type"))
			var body httpError
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
			assert.Equal(t, errors.UnclassifiedCode, body.Code)
			assert.Equal(t, tt.message, body.Message)
			assert.Equal(t, map[string]any{}, body.Details)
		})
	}
}
