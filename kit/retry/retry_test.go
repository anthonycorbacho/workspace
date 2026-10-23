package retry

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/anthonycorbacho/workspace/kit/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type testTimer struct {
	timer *time.Timer
}

func (t *testTimer) Start(duration time.Duration) {
	t.timer = time.NewTimer(0)
}

func (t *testTimer) Stop() {
	if t.timer != nil {
		t.timer.Stop()
	}
}

func (t *testTimer) C() <-chan time.Time {
	return t.timer.C
}

func TestRetry(t *testing.T) {
	const successOn = 3
	i := 0

	// This function is successful on "successOn" calls.
	f := func() error {
		i++
		if i == successOn {
			return nil
		}
		return errors.New("error")
	}

	err := Retry(context.Background(), f, NewExponentialBackOff())
	if err != nil {
		t.Errorf("unexpected error: %s", err.Error())
	}
	if i != successOn {
		t.Errorf("invalid number of retries: %d", i)
	}
}

func TestRetryContext(t *testing.T) {
	cancelOn := 3
	i := 0

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// This function cancels context on "cancelOn" calls.
	f := func() error {
		i++
		// cancelling the context in the operation function is not a typical
		// use-case, however it allows to get predictable test results.
		if i == cancelOn {
			cancel()
		}
		return fmt.Errorf("error (%d)", i)
	}

	err := Retry(ctx, f, NewConstantBackOff(time.Millisecond))
	if err == nil {
		t.Errorf("error is unexpectedly nil")
	}
	if !errors.Is(err, context.Canceled) {
		t.Errorf("unexpected error: %s", err.Error())
	}
	if i != cancelOn {
		t.Errorf("invalid number of retries: %d", i)
	}
}

func TestRetryOutcome(t *testing.T) {
	errOp := errors.New("operation failed")

	tests := []struct {
		name      string
		failures  int // number of calls failing before success, -1 always fails.
		opErr     error
		backoff   func() BackOff
		wantCalls int
		wantErr   []error // errors expected in the returned chain, nil means success.
	}{
		{
			name:      "success on first call",
			failures:  0,
			backoff:   func() BackOff { return NewConstantBackOff(time.Hour) },
			wantCalls: 1,
		},
		{
			name:      "success after failures",
			failures:  4,
			opErr:     errOp,
			backoff:   func() BackOff { return NewConstantBackOff(time.Millisecond) },
			wantCalls: 5,
		},
		{
			name:      "success with zero backoff",
			failures:  3,
			opErr:     errOp,
			backoff:   func() BackOff { return NewConstantBackOff(0) },
			wantCalls: 4,
		},
		{
			name:      "permanent error is not retried",
			failures:  -1,
			opErr:     errors.Wrap(ErrPermanent, "boom"),
			backoff:   func() BackOff { return NewConstantBackOff(time.Hour) },
			wantCalls: 1,
			wantErr:   []error{ErrPermanent},
		},
		{
			name:      "permanent error is returned as is",
			failures:  -1,
			opErr:     ErrPermanent,
			backoff:   func() BackOff { return NewConstantBackOff(time.Hour) },
			wantCalls: 1,
			wantErr:   []error{ErrPermanent},
		},
		{
			name:      "backoff stop returns permanent error",
			failures:  -1,
			opErr:     errOp,
			backoff:   func() BackOff { return NewConstantBackOff(Stop) },
			wantCalls: 1,
			wantErr:   []error{ErrPermanent},
		},
		{
			name:     "max elapsed time reached",
			failures: -1,
			opErr:    errOp,
			backoff: func() BackOff {
				// every call to Now() moves the clock by 1s, so the 5th
				// NextBackOff() exceeds the 5s max elapsed time.
				return NewExponentialBackOff(
					WithInitialInterval(time.Millisecond),
					WithRandomizationFactor(0),
					WithMultiplier(1),
					WithMaxElapsedTime(5*time.Second),
					WithClockProvider(&TestClock{}),
				)
			},
			wantCalls: 5,
			wantErr:   []error{ErrPermanent},
		},
		{
			name:     "exponential backoff success",
			failures: 3,
			opErr:    errOp,
			backoff: func() BackOff {
				return NewExponentialBackOff(
					WithInitialInterval(time.Millisecond),
					WithMaxInterval(5*time.Millisecond),
				)
			},
			wantCalls: 4,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			calls := 0
			op := func() error {
				calls++
				if tt.failures < 0 || calls <= tt.failures {
					return tt.opErr
				}
				return nil
			}

			err := Retry(context.Background(), op, tt.backoff())
			assert.Equal(t, tt.wantCalls, calls)
			if len(tt.wantErr) == 0 {
				assert.NoError(t, err)
				return
			}
			require.Error(t, err)
			assert.ErrorIs(t, err, tt.opErr, "the operation error is kept")
			for _, want := range tt.wantErr {
				assert.ErrorIs(t, err, want)
			}
		})
	}
}

func TestRetryContextDone(t *testing.T) {
	errOp := errors.New("operation failed")

	tests := []struct {
		name    string
		ctx     func(t *testing.T) context.Context
		backoff BackOff
		wantErr error
	}{
		{
			name: "context already cancelled",
			ctx: func(t *testing.T) context.Context {
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				return ctx
			},
			backoff: NewConstantBackOff(time.Hour),
			wantErr: context.Canceled,
		},
		{
			name: "context cancelled and backoff stop",
			ctx: func(t *testing.T) context.Context {
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				return ctx
			},
			backoff: NewConstantBackOff(Stop),
			wantErr: context.Canceled,
		},
		{
			name: "context deadline exceeded while waiting",
			ctx: func(t *testing.T) context.Context {
				ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
				t.Cleanup(cancel)
				return ctx
			},
			backoff: NewConstantBackOff(time.Hour),
			wantErr: context.DeadlineExceeded,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			calls := 0
			op := func() error {
				calls++
				return errOp
			}

			err := Retry(tt.ctx(t), op, tt.backoff)
			// the operation is always run at least once.
			assert.Equal(t, 1, calls)
			require.Error(t, err)
			assert.ErrorIs(t, err, tt.wantErr)
			assert.ErrorIs(t, err, errOp, "the operation error is kept")
		})
	}
}

func TestDoRetryWithTimer(t *testing.T) {
	calls := 0
	op := func() error {
		calls++
		if calls < 3 {
			return errors.New("error")
		}
		return nil
	}

	// a test timer firing immediately allows to retry with a long backoff.
	err := doRetry(context.Background(), op, NewConstantBackOff(time.Hour), &testTimer{})
	assert.NoError(t, err)
	assert.Equal(t, 3, calls)
}

func TestError(t *testing.T) {
	assert.Equal(t, "permanent", ErrPermanent.Error())
	assert.Equal(t, "custom", Error("custom").Error())
}
