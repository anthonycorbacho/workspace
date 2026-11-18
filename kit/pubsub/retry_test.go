package pubsub

import (
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestPermanent(t *testing.T) {
	cause := errors.New("malformed payload")
	tests := []struct {
		name          string
		err           error
		wantPermanent bool
	}{
		{name: "nil", err: nil},
		{name: "plain error", err: cause},
		{name: "permanent", err: Permanent(cause), wantPermanent: true},
		{name: "wrapped permanent", err: fmt.Errorf("decoding order: %w", Permanent(cause)), wantPermanent: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.wantPermanent, IsPermanent(tt.err))
		})
	}

	assert.NoError(t, Permanent(nil))
	err := Permanent(cause)
	assert.ErrorIs(t, err, cause, "the cause stays in the chain")
	assert.Equal(t, cause.Error(), err.Error())
}

func TestBackoffDelay(t *testing.T) {
	b := Backoff{Initial: time.Second, Max: 5 * time.Second}
	tests := []struct {
		name    string
		backoff Backoff
		attempt int
		want    time.Duration
	}{
		{name: "first attempt", backoff: b, attempt: 1, want: time.Second},
		{name: "doubles", backoff: b, attempt: 3, want: 4 * time.Second},
		{name: "capped by max", backoff: b, attempt: 4, want: 5 * time.Second},
		{name: "many attempts don't overflow", backoff: b, attempt: 1000, want: 5 * time.Second},
		{name: "no max", backoff: Backoff{Initial: time.Second}, attempt: 4, want: 8 * time.Second},
		{name: "no max doesn't overflow", backoff: Backoff{Initial: time.Second}, attempt: 1000, want: time.Second << 33},
		{name: "zero backoff redelivers right away", backoff: Backoff{}, attempt: 3, want: 0},
		{name: "invalid attempt", backoff: b, attempt: 0, want: 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, tt.backoff.Delay(tt.attempt))
		})
	}
}
