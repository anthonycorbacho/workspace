package grpc

import (
	stderrors "errors"
	"fmt"
	"strings"
	"testing"

	errdetailsv1 "github.com/anthonycorbacho/workspace/api/errdetails/v1"
	"github.com/anthonycorbacho/workspace/kit/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestKindCodesAreExhaustive(t *testing.T) {
	seen := map[codes.Code]errors.Kind{}
	for k := errors.KindUnknown; k <= errors.KindDataLoss; k++ {
		c, ok := _kindCodes[k]
		require.True(t, ok, "kind %s has no gRPC code", k)
		// the kinds are named after the gRPC codes (eg: NOT_FOUND and NotFound).
		assert.Equal(t, strings.ReplaceAll(k.String(), "_", ""), strings.ToUpper(strings.Replace(c.String(), "Canceled", "Cancelled", 1)))
		_, dup := seen[c]
		assert.False(t, dup, "gRPC code %s mapped twice", c)
		seen[c] = k
		assert.Equal(t, k, _codeKinds[c])
	}
}

func TestStatus(t *testing.T) {
	type custom struct{ v int }
	tests := []struct {
		name        string
		err         *errors.Error
		wantCode    codes.Code
		wantDetails map[string]any
	}{
		{name: "coded error with details", err: errors.SampleUserNotFound("42"), wantCode: codes.NotFound, wantDetails: map[string]any{"id": "42"}},
		{name: "unsupported detail value is converted to string", err: errors.Internal().WithDetail("custom", custom{v: 1}), wantCode: codes.Internal, wantDetails: map[string]any{"custom": "{1}"}},
		{name: "context kind", err: errors.NewUnclassified(errors.KindCanceled, "context canceled"), wantCode: codes.Canceled, wantDetails: map[string]any{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			st := Status(tt.err)
			assert.Equal(t, tt.wantCode, st.Code())
			assert.Equal(t, tt.err.Message, st.Message())
			require.Len(t, st.Details(), 1)
			detail, ok := st.Details()[0].(*errdetailsv1.Error)
			require.True(t, ok)
			assert.Equal(t, tt.err.Code, detail.GetCode())
			assert.Equal(t, tt.wantDetails, detail.GetDetails().AsMap())
		})
	}
}

func TestStatusDoesNotLeakCause(t *testing.T) {
	err := errors.Internal().WithCause(stderrors.New("password=secret"))
	st := Status(err)
	assert.Equal(t, "internal error", st.Message())
	assert.NotContains(t, fmt.Sprint(st.Proto()), "secret")
}

func TestFromStatusAndFromError(t *testing.T) {
	tests := []struct {
		name        string
		err         error
		wantNil     bool
		wantCode    string
		wantKind    errors.Kind
		wantMessage string
		wantDetails map[string]any
	}{
		{name: "nil", err: nil, wantNil: true},
		{
			name: "coded error received from another service", err: Status(errors.SampleUserNotFound("1")).Err(),
			wantCode: errors.SampleUserNotFoundCode, wantKind: errors.KindNotFound, wantMessage: "user 1 not found", wantDetails: map[string]any{"id": "1"},
		},
		{
			name: "coded error", err: fmt.Errorf("fetch: %w", errors.SampleUserNotFound("1")),
			wantCode: errors.SampleUserNotFoundCode, wantKind: errors.KindNotFound, wantMessage: "user 1 not found", wantDetails: map[string]any{"id": "1"},
		},
		{
			name: "status without coded error keeps its kind and message", err: status.Error(codes.FailedPrecondition, "bucket is not empty"),
			wantCode: errors.UnclassifiedCode, wantKind: errors.KindFailedPrecondition, wantMessage: "bucket is not empty", wantDetails: map[string]any{},
		},
		{
			name: "internal status message is not kept", err: status.Error(codes.Internal, "panic at db.go:42"),
			wantCode: errors.InternalCode, wantKind: errors.KindInternal, wantMessage: "internal error", wantDetails: map[string]any{},
		},
		{
			name: "plain error is internal", err: stderrors.New("connection refused"),
			wantCode: errors.InternalCode, wantKind: errors.KindInternal, wantMessage: "internal error", wantDetails: map[string]any{},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FromError(tt.err)
			if tt.wantNil {
				assert.Nil(t, got)
				return
			}
			require.NotNil(t, got)
			assert.Equal(t, tt.wantCode, got.Code)
			assert.Equal(t, tt.wantKind, got.Kind)
			assert.Equal(t, tt.wantMessage, got.Message)
			assert.Equal(t, tt.wantDetails, got.Details)
		})
	}
	assert.Nil(t, FromStatus(status.New(codes.OK, "")))
}
