package errors

import (
	"context"
	stderrors "errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestErrorString(t *testing.T) {
	cause := stderrors.New("db: no rows")
	tests := []struct {
		name string
		err  *Error
		want string
	}{
		{name: "code and message", err: SampleUserNotFound("42"), want: "ERR_01001: user 42 not found"},
		{name: "with cause", err: SampleUserNotFound("42").WithCause(cause), want: "ERR_01001: user 42 not found: db: no rows"},
		{
			name: "with internal details sorted",
			err:  &Error{Code: "ERR_99999", Message: "failed", internal: map[string]any{"b": 2, "a": "1"}},
			want: "ERR_99999: failed (a=1, b=2)",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, tt.err.Error())
		})
	}
}

func TestErrorIs(t *testing.T) {
	cause := stderrors.New("db: no rows")
	tests := []struct {
		name   string
		err    error
		target error
		want   bool
	}{
		{name: "same code different values", err: SampleUserNotFound("1"), target: SampleUserNotFound("2"), want: true},
		{name: "different code", err: SampleUserNotFound("1"), target: SampleUserAlreadyExists("1"), want: false},
		{name: "wrapped coded error", err: fmt.Errorf("fetch: %w", SampleUserNotFound("1")), target: SampleUserNotFound(""), want: true},
		{name: "cause is in the chain", err: SampleUserNotFound("1").WithCause(cause), target: cause, want: true},
		{name: "plain error", err: cause, target: SampleUserNotFound(""), want: false},
		{name: "nil target", err: SampleUserNotFound("1"), target: (*Error)(nil), want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, Is(tt.err, tt.target))
		})
	}
}

func TestErrorImmutable(t *testing.T) {
	original := SampleUserNotFound("1")
	withDetail := original.WithDetail("tenant", "acme")
	withCause := original.WithCause(stderrors.New("boom"))

	assert.NotContains(t, original.Details, "tenant", "WithDetail must not change the original error")
	assert.Nil(t, original.Unwrap(), "WithCause must not change the original error")
	assert.Equal(t, "acme", withDetail.Details["tenant"])
	assert.NotNil(t, withCause.Unwrap())

	// a zero value error can be enriched.
	assert.Equal(t, "v", (&Error{}).WithDetail("k", "v").Details["k"])
}

func TestFromError(t *testing.T) {
	plain := stderrors.New("dial tcp 10.0.0.1:5432: connection refused")
	tests := []struct {
		name        string
		err         error
		wantNil     bool
		wantCode    string
		wantKind    Kind
		wantMessage string
		wantDetails map[string]any
	}{
		{name: "nil", err: nil, wantNil: true},
		{
			name:     "coded error",
			err:      SampleUserNotFound("1"),
			wantCode: SampleUserNotFoundCode, wantKind: KindNotFound, wantMessage: "user 1 not found",
			wantDetails: map[string]any{"id": "1"},
		},
		{
			name:     "wrapped coded error",
			err:      fmt.Errorf("fetch: %w", SampleUserAlreadyExists("bob")),
			wantCode: SampleUserAlreadyExistsCode, wantKind: KindAlreadyExists, wantMessage: "user bob already exists",
			wantDetails: map[string]any{"name": "bob"},
		},
		{
			name:     "plain error is internal",
			err:      plain,
			wantCode: InternalCode, wantKind: KindInternal, wantMessage: "internal error",
			wantDetails: map[string]any{},
		},
		{
			name:     "context canceled",
			err:      fmt.Errorf("fetch: %w", context.Canceled),
			wantCode: UnclassifiedCode, wantKind: KindCanceled, wantMessage: context.Canceled.Error(),
			wantDetails: map[string]any{},
		},
		{
			name:     "context deadline exceeded",
			err:      context.DeadlineExceeded,
			wantCode: UnclassifiedCode, wantKind: KindDeadlineExceeded, wantMessage: context.DeadlineExceeded.Error(),
			wantDetails: map[string]any{},
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
}

func TestFromErrorKeepsCause(t *testing.T) {
	plain := stderrors.New("connection refused")
	got := FromError(plain)
	assert.ErrorIs(t, got, plain, "the original error is kept as cause")
	assert.Contains(t, got.Error(), "connection refused", "the cause is part of the error string for logs")
}

func TestHasCode(t *testing.T) {
	cause := SampleUserAlreadyExists("bob")
	tests := []struct {
		name string
		err  error
		code string
		want bool
	}{
		{name: "same code", err: SampleUserNotFound("1"), code: SampleUserNotFoundCode, want: true},
		{name: "wrapped", err: fmt.Errorf("fetch: %w", SampleUserNotFound("1")), code: SampleUserNotFoundCode, want: true},
		{name: "coded cause", err: InvalidRequest().WithCause(cause), code: SampleUserAlreadyExistsCode, want: true},
		{name: "different code", err: SampleUserNotFound("1"), code: InternalCode, want: false},
		{name: "plain error", err: stderrors.New("boom"), code: InternalCode, want: false},
		{name: "nil", err: nil, code: InternalCode, want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, HasCode(tt.err, tt.code))
		})
	}
}

func TestKindString(t *testing.T) {
	assert.Equal(t, "NOT_FOUND", KindNotFound.String())
	assert.Equal(t, "CANCELLED", KindCanceled.String())
	assert.Equal(t, "UNKNOWN", Kind(-1).String())
}
